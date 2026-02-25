package generation

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"mini-atoms/internal/spec"
)

type StubClient struct{}

func NewStubClient() *StubClient {
	return &StubClient{}
}

func (c *StubClient) GenerateSpecJSON(_ context.Context, req ClientRequest) (string, error) {
	log.Printf("stub generation client invoked: prompt_chars=%d has_current_draft=%t has_repair_error=%t", len([]rune(strings.TrimSpace(req.UserPrompt))), strings.TrimSpace(req.CurrentDraftJSON) != "", strings.TrimSpace(req.RepairError) != "")
	appName := summarizePromptToAppName(req.UserPrompt)
	collectionName := "todos"
	blocks := []spec.BlockSpec{
		{Type: "form", Collection: collectionName},
		{Type: "list", Collection: collectionName},
		{Type: "toggle", Collection: collectionName, Field: "done"},
		{Type: "stats", Collection: collectionName, Metric: "count", Label: "总数"},
	}

	if containsAny(req.UserPrompt, "计时", "番茄", "focus", "pomodoro") {
		collectionName = "focus_sessions"
		blocks = []spec.BlockSpec{
			{Type: "timer", SessionCollection: collectionName, WorkMinutes: 25, BreakMinutes: 5},
			{Type: "list", Collection: collectionName},
			{Type: "stats", Collection: collectionName, Metric: "sum", Field: "minutes", Label: "总分钟数"},
		}
	}

	fields := []spec.FieldSpec{
		{Name: "title", Type: spec.FieldTypeText, Required: true},
		{Name: "done", Type: spec.FieldTypeBool, Required: true},
	}
	if collectionName == "focus_sessions" {
		fields = []spec.FieldSpec{
			{Name: "task", Type: spec.FieldTypeText, Required: true},
			{Name: "minutes", Type: spec.FieldTypeInt, Required: true},
			{Name: "completed_at", Type: spec.FieldTypeDatetime},
		}
	}

	appSpec := spec.AppSpec{
		AppName: appName,
		Theme:   "light",
		Collections: []spec.CollectionSpec{
			{
				Name:   collectionName,
				Fields: fields,
			},
		},
		Pages: []spec.PageSpec{
			{
				ID:     "home",
				Title:  "Home",
				Blocks: blocks,
			},
		},
	}

	b, _ := json.Marshal(appSpec)
	return string(b), nil
}

func (c *StubClient) GenerateProjectName(_ context.Context, goalPrompt string) (string, error) {
	log.Printf("stub project name generator invoked: prompt_chars=%d", len([]rune(strings.TrimSpace(goalPrompt))))
	return summarizePromptToAppName(goalPrompt), nil
}

func summarizePromptToAppName(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Generated App"
	}
	if containsAny(prompt, "简历", "resume") {
		return "Resume Builder"
	}
	if containsAny(prompt, "待办", "todo") {
		return "Todo App"
	}
	if containsAny(prompt, "crm", "客户") {
		return "CRM Dashboard"
	}
	return "Generated App"
}

func containsAny(s string, subs ...string) bool {
	ls := strings.ToLower(s)
	for _, sub := range subs {
		if strings.Contains(ls, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
