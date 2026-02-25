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
