package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	defaultDeepSeekBaseURL = "https://api.deepseek.com"
	defaultDeepSeekModel   = "deepseek-chat"
)

type DeepSeekClientConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	AppBaseURL  string
	HTTPClient  *http.Client
	HTTPTimeout time.Duration
}

type DeepSeekClient struct {
	apiKey     string
	baseURL    string
	model      string
	appBaseURL string
	httpClient *http.Client
}

func NewDeepSeekClient(cfg DeepSeekClientConfig) *DeepSeekClient {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultDeepSeekBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaultDeepSeekModel
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.HTTPTimeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		httpClient = &http.Client{Timeout: timeout}
	}
	return &DeepSeekClient{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		appBaseURL: strings.TrimSpace(cfg.AppBaseURL),
		httpClient: httpClient,
	}
}

func (c *DeepSeekClient) GenerateSpecJSON(ctx context.Context, req ClientRequest) (string, error) {
	if c == nil {
		return "", fmt.Errorf("deepseek client is nil")
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return "", fmt.Errorf("deepseek api key is empty")
	}
	if strings.TrimSpace(req.UserPrompt) == "" {
		return "", fmt.Errorf("deepseek request user prompt is empty")
	}
	startedAt := time.Now()
	log.Printf("deepseek generation request started: model=%s prompt_chars=%d has_current_draft=%t has_repair_error=%t", c.model, len([]rune(strings.TrimSpace(req.UserPrompt))), strings.TrimSpace(req.CurrentDraftJSON) != "", strings.TrimSpace(req.RepairError) != "")

	body := map[string]any{
		"model":       c.model,
		"temperature": 0.2,
		"messages":    c.buildMessages(req),
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal deepseek request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build deepseek request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		log.Printf("deepseek generation request failed: model=%s duration_ms=%d err=%v", c.model, time.Since(startedAt).Milliseconds(), err)
		return "", fmt.Errorf("call deepseek: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read deepseek response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := parseDeepSeekErrorMessage(respBody)
		if msg == "" {
			msg = strings.TrimSpace(string(respBody))
		}
		log.Printf("deepseek generation response error: model=%s status=%d duration_ms=%d", c.model, resp.StatusCode, time.Since(startedAt).Milliseconds())
		return "", fmt.Errorf("deepseek returned status %d: %s", resp.StatusCode, msg)
	}

	var parsed deepSeekChatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse deepseek response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("deepseek response has no choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("deepseek response choice content is empty")
	}

	content = stripMarkdownCodeFence(content)
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("deepseek response content is empty after cleanup")
	}
	content = unwrapSpecEnvelopeJSON(content)
	content = normalizeSpecAliasesJSON(content)
	log.Printf("deepseek generation request succeeded: model=%s status=%d duration_ms=%d response_chars=%d", c.model, resp.StatusCode, time.Since(startedAt).Milliseconds(), len([]rune(content)))
	return content, nil
}

func (c *DeepSeekClient) buildMessages(req ClientRequest) []deepSeekMessage {
	systemPrompt := strings.TrimSpace(`
你是 mini-atoms 的 Spec 生成器。你必须只输出 JSON（不要 markdown 代码块、不要解释文字）。
返回一个完整的 app spec，用于 mini-atoms 渲染。

输出格式（严格）：
- 输出必须是单个 JSON 对象，不要前后缀文本，不要代码块，不要注释
- 顶层必须包含 app_name、collections、pages
- 顶层不要把结果包在 spec/result/data 等额外对象里
- 所有 key 使用 snake_case（例如 app_name、page_id、session_collection）

结构约束：
- 允许 blocks: nav, list, form, toggle, stats, timer
- 允许字段类型: text, int, real, bool, date, datetime, enum
- collections <= 5；每个 collection 字段 <= 10
- list/form/toggle/stats 每个 block 都必须显式包含 collection（不能省略），即使同页其他 block 已写过
- toggle.field 必须引用 bool 字段
- stats.metric 只允许 count 或 sum
- stats.metric = sum 时 field 必须是 int 或 real
- enum 字段必须包含非空 options
- pages 和 collections 必须完整可用
- nav.items[*].page_id 必须引用已存在的 pages[*].id
- 如果收到修复错误信息，必须逐项修复后重新输出完整 JSON（不是差量）
`)

	var userBuilder strings.Builder
	userBuilder.WriteString("用户需求：\n")
	userBuilder.WriteString(req.UserPrompt)

	if strings.TrimSpace(req.CurrentDraftJSON) != "" {
		userBuilder.WriteString("\n\n当前草稿（可参考并重写为完整 JSON）：\n")
		userBuilder.WriteString(req.CurrentDraftJSON)
	}
	if strings.TrimSpace(req.RepairError) != "" {
		userBuilder.WriteString("\n\n上一次输出存在错误，请修复并重新输出完整 JSON：\n")
		userBuilder.WriteString(req.RepairError)
	}
	if c.appBaseURL != "" {
		userBuilder.WriteString("\n\n上下文：应用运行在 ")
		userBuilder.WriteString(c.appBaseURL)
	}

	return []deepSeekMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userBuilder.String()},
	}
}

type deepSeekMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type deepSeekChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func parseDeepSeekErrorMessage(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Error.Message)
}

func stripMarkdownCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[idx+1:]
	}
	if end := strings.LastIndex(s, "```"); end >= 0 {
		s = s[:end]
	}
	return strings.TrimSpace(s)
}

func unwrapSpecEnvelopeJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return s
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return s
	}

	// If top-level already looks like spec, keep as-is.
	if _, hasCollections := raw["collections"]; hasCollections {
		if _, hasPages := raw["pages"]; hasPages {
			return s
		}
	}

	for _, key := range []string{"spec", "result", "data"} {
		v, ok := raw[key]
		if !ok {
			continue
		}
		var inner map[string]json.RawMessage
		if err := json.Unmarshal(v, &inner); err != nil {
			continue
		}
		if _, hasCollections := inner["collections"]; !hasCollections {
			continue
		}
		if _, hasPages := inner["pages"]; !hasPages {
			continue
		}
		return strings.TrimSpace(string(v))
	}

	return s
}

func normalizeSpecAliasesJSON(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return s
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(s), &root); err != nil {
		return s
	}

	if _, ok := root["app_name"]; !ok {
		if name, ok := root["name"].(string); ok && strings.TrimSpace(name) != "" {
			root["app_name"] = strings.TrimSpace(name)
		}
	}

	if pages, ok := root["pages"].([]any); ok {
		for i, p := range pages {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			moveAliasValue(pm, "blocks", "components")
			if _, hasID := pm["id"]; !hasID {
				if name, ok := pm["name"].(string); ok && strings.TrimSpace(name) != "" {
					pm["id"] = strings.TrimSpace(name)
				}
			}
			if _, hasTitle := pm["title"]; !hasTitle {
				if label, ok := pm["label"].(string); ok && strings.TrimSpace(label) != "" {
					pm["title"] = strings.TrimSpace(label)
				}
			}
			normalizeStringAlias(pm, "id", "page_id", "pageId")
			normalizeStringAlias(pm, "title", "page_title")

			if blocks, ok := pm["blocks"].([]any); ok {
				for j, b := range blocks {
					bm, ok := b.(map[string]any)
					if !ok {
						continue
					}

					normalizeStringAlias(bm, "type", "kind", "block_type", "blockType")
					normalizeStringAlias(bm, "collection", "source", "dataset", "collection_name", "collectionName", "table")
					normalizeStringAlias(bm, "field", "column", "field_name", "fieldName")
					normalizeStringAlias(bm, "metric", "metric_type", "metricType", "aggregation")
					normalizeStringAlias(bm, "session_collection", "sessionCollection", "session_collection_name")
					moveAliasValue(bm, "items", "links", "nav_items", "navItems")

					if items, ok := bm["items"].([]any); ok {
						for k, item := range items {
							im, ok := item.(map[string]any)
							if !ok {
								continue
							}
							normalizeStringAlias(im, "page_id", "pageId", "page")
							items[k] = im
						}
						bm["items"] = items
					}

					blocks[j] = bm
				}
				pm["blocks"] = blocks
			}
			pages[i] = pm
		}
		root["pages"] = pages
	}

	normalized, err := json.Marshal(root)
	if err != nil {
		return s
	}
	return string(normalized)
}

func moveAliasValue(m map[string]any, target string, aliases ...string) {
	if m == nil {
		return
	}
	if _, ok := m[target]; ok {
		for _, alias := range aliases {
			delete(m, alias)
		}
		return
	}
	for _, alias := range aliases {
		if v, ok := m[alias]; ok {
			m[target] = v
			break
		}
	}
	for _, alias := range aliases {
		delete(m, alias)
	}
}

func normalizeStringAlias(m map[string]any, target string, aliases ...string) {
	if m == nil {
		return
	}

	if v, ok := m[target]; ok {
		if s, ok := v.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				m[target] = trimmed
				for _, alias := range aliases {
					delete(m, alias)
				}
				return
			}
		} else {
			for _, alias := range aliases {
				delete(m, alias)
			}
			return
		}
	}

	for _, alias := range aliases {
		raw, ok := m[alias]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			m[target] = trimmed
			break
		}
	}
	for _, alias := range aliases {
		delete(m, alias)
	}
}
