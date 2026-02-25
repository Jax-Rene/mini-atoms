package generation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeepSeekClient_GenerateSpecJSON_Success(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(bodyBytes, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		content := "```json\n" +
			"{\"app_name\":\"Todo App\",\"collections\":[{\"name\":\"todos\",\"fields\":[{\"name\":\"title\",\"type\":\"text\"},{\"name\":\"done\",\"type\":\"bool\"}]}],\"pages\":[{\"id\":\"home\",\"blocks\":[{\"type\":\"list\",\"collection\":\"todos\"},{\"type\":\"toggle\",\"collection\":\"todos\",\"field\":\"done\"},{\"type\":\"stats\",\"collection\":\"todos\",\"metric\":\"count\"}]}]}\n" +
			"```"
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": content,
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:      "test-key",
		BaseURL:     srv.URL,
		Model:       "deepseek-chat",
		HTTPClient:  srv.Client(),
		AppBaseURL:  "http://localhost:8080",
		HTTPTimeout: 2 * time.Second,
	})

	got, err := client.GenerateSpecJSON(context.Background(), ClientRequest{
		UserPrompt:       "做一个待办应用",
		CurrentDraftJSON: `{"app_name":"Old"}`,
	})
	if err != nil {
		t.Fatalf("GenerateSpecJSON() error = %v", err)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want bearer token", gotAuth)
	}
	if gotBody["model"] != "deepseek-chat" {
		t.Fatalf("model = %#v", gotBody["model"])
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("messages malformed: %#v", gotBody["messages"])
	}
	if !strings.Contains(got, `"app_name":"Todo App"`) {
		t.Fatalf("response content not normalized JSON: %q", got)
	}
	if strings.Contains(got, "```") {
		t.Fatalf("response still contains markdown fence: %q", got)
	}
}

func TestDeepSeekClient_GenerateSpecJSON_ErrorResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:     "bad-key",
		BaseURL:    srv.URL,
		Model:      "deepseek-chat",
		HTTPClient: srv.Client(),
	})

	_, err := client.GenerateSpecJSON(context.Background(), ClientRequest{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 401") || !strings.Contains(strings.ToLower(err.Error()), "invalid api key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeepSeekClient_GenerateSpecJSON_EmptyChoiceContent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":""}}]}`))
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Model:      "deepseek-chat",
		HTTPClient: srv.Client(),
	})

	_, err := client.GenerateSpecJSON(context.Background(), ClientRequest{UserPrompt: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeepSeekClient_GenerateSpecJSON_UnwrapsSpecEnvelope(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[
				{
					"message":{
						"content":"{\"spec\":{\"app_name\":\"Todo App\",\"collections\":[{\"name\":\"todos\",\"fields\":[{\"name\":\"title\",\"type\":\"text\"},{\"name\":\"done\",\"type\":\"bool\"}]}],\"pages\":[{\"id\":\"home\",\"blocks\":[{\"type\":\"list\",\"collection\":\"todos\"},{\"type\":\"toggle\",\"collection\":\"todos\",\"field\":\"done\"},{\"type\":\"stats\",\"collection\":\"todos\",\"metric\":\"count\"}]}]}}"
					}
				}
			]
		}`))
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		Model:      "deepseek-chat",
		HTTPClient: srv.Client(),
	})

	got, err := client.GenerateSpecJSON(context.Background(), ClientRequest{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("GenerateSpecJSON() error = %v", err)
	}
	if strings.Contains(got, `"spec"`) {
		t.Fatalf("expected unwrapped spec content, got %q", got)
	}
	if !strings.Contains(got, `"collections"`) || !strings.Contains(got, `"pages"`) {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestDeepSeekClient_GenerateSpecJSON_NormalizesCommonAliases(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[
				{
					"message":{
						"content":"{\"name\":\"待办应用\",\"collections\":[{\"name\":\"todos\",\"fields\":[{\"name\":\"title\",\"type\":\"text\"},{\"name\":\"done\",\"type\":\"bool\"}]}],\"pages\":[{\"name\":\"dashboard\",\"label\":\"仪表板\",\"blocks\":[{\"type\":\"list\",\"collection\":\"todos\"},{\"type\":\"toggle\",\"collection\":\"todos\",\"field\":\"done\"},{\"type\":\"stats\",\"collection\":\"todos\",\"metric\":\"count\"}]}]}"
					}
				}
			]
		}`))
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})

	got, err := client.GenerateSpecJSON(context.Background(), ClientRequest{UserPrompt: "test"})
	if err != nil {
		t.Fatalf("GenerateSpecJSON() error = %v", err)
	}
	if !strings.Contains(got, `"app_name":"待办应用"`) {
		t.Fatalf("expected app_name alias normalized, got %q", got)
	}
	if !strings.Contains(got, `"id":"dashboard"`) {
		t.Fatalf("expected page.id alias normalized, got %q", got)
	}
	if !strings.Contains(got, `"title":"仪表板"`) {
		t.Fatalf("expected page.title alias normalized, got %q", got)
	}
}

func TestDeepSeekClient_GenerateProjectName_Success(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(bodyBytes, &gotBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```text\n项目名：AI 看板助手\n```",
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		APIKey:     "test-key",
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
	})

	got, err := client.GenerateProjectName(context.Background(), "帮我做一个简单看板")
	if err != nil {
		t.Fatalf("GenerateProjectName() error = %v", err)
	}
	if got != "AI 看板助手" {
		t.Fatalf("GenerateProjectName() = %q, want %q", got, "AI 看板助手")
	}
	msgs, ok := gotBody["messages"].([]any)
	if !ok || len(msgs) < 2 {
		t.Fatalf("messages malformed: %#v", gotBody["messages"])
	}
	userMsg, _ := msgs[1].(map[string]any)
	content, _ := userMsg["content"].(string)
	if !strings.Contains(content, "帮我做一个简单看板") {
		t.Fatalf("user message missing goal prompt: %q", content)
	}
}

func TestDeepSeekClient_buildMessages_IncludesStrictOutputFormatRules(t *testing.T) {
	t.Parallel()

	client := NewDeepSeekClient(DeepSeekClientConfig{
		AppBaseURL: "http://localhost:8080",
	})

	msgs := client.buildMessages(ClientRequest{
		UserPrompt:       "做一个待办应用",
		CurrentDraftJSON: `{"app_name":"Old"}`,
		RepairError:      `validate spec: page "dashboard" block[2] toggle.collection is required`,
	})
	if len(msgs) != 2 {
		t.Fatalf("messages len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Fatalf("system role = %q", msgs[0].Role)
	}
	systemPrompt := msgs[0].Content
	for _, want := range []string{
		"单个 JSON 对象",
		"顶层必须包含 app_name、collections、pages",
		"不要把结果包在 spec/result/data",
		"list/form/toggle/stats",
		"必须显式包含 collection（不能省略）",
		"默认不要再额外生成同字段的 toggle block",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, systemPrompt)
		}
	}

	userPrompt := msgs[1].Content
	for _, want := range []string{
		"当前草稿（可参考并重写为完整 JSON）",
		"上一次输出存在错误，请修复并重新输出完整 JSON",
		`toggle.collection is required`,
		"http://localhost:8080",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestNormalizeSpecAliasesJSON_NormalizesBlockAliases(t *testing.T) {
	t.Parallel()

	raw := `{
		"name":"待办应用",
		"collections":[{"name":"todos","fields":[{"name":"title","type":"text"},{"name":"done","type":"bool"}]}],
		"pages":[
			{
				"name":"dashboard",
				"components":[
					{"kind":"list","source":"todos"},
					{"kind":"toggle","source":"todos","field":"done"},
					{"kind":"nav","items":[{"label":"首页","pageId":"dashboard"}]}
				]
			}
		]
	}`

	got := normalizeSpecAliasesJSON(raw)

	for _, want := range []string{
		`"app_name":"待办应用"`,
		`"id":"dashboard"`,
		`"blocks":[`,
		`"type":"list"`,
		`"collection":"todos"`,
		`"page_id":"dashboard"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("normalized JSON missing %q: %s", want, got)
		}
	}
	for _, bad := range []string{`"components"`, `"kind"`, `"source"`, `"pageId"`} {
		if strings.Contains(got, bad) {
			t.Fatalf("normalized JSON still contains alias %q: %s", bad, got)
		}
	}
}

func TestNormalizeSpecAliasesJSON_DefaultsEmptyStatsMetricToCount(t *testing.T) {
	t.Parallel()

	raw := `{
		"app_name":"输入统计",
		"collections":[{"name":"inputs","fields":[{"name":"title","type":"text"}]}],
		"pages":[
			{
				"id":"page_input_stats",
				"blocks":[
					{"type":"list","collection":"inputs"},
					{"type":"stats","collection":"inputs","metric":"   ","label":"总数"}
				]
			}
		]
	}`

	got := normalizeSpecAliasesJSON(raw)

	if !strings.Contains(got, `"type":"stats"`) {
		t.Fatalf("normalized JSON missing stats block: %s", got)
	}
	if !strings.Contains(got, `"metric":"count"`) {
		t.Fatalf("expected empty stats.metric to default to count, got %s", got)
	}
}
