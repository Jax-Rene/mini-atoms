package store

import (
	"strings"
	"testing"
)

func TestGenerateProjectNameFromPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prompt string
	}{
		{
			name:   "chinese prompt",
			prompt: "帮我做一个简历网站，支持项目经历、作品展示和导出 PDF。",
		},
		{
			name:   "english prompt",
			prompt: "Build a landing page for a SaaS analytics product with pricing and testimonials",
		},
		{
			name:   "empty fallback",
			prompt: "   ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := generateProjectNameFromPrompt(tt.prompt)
			if strings.TrimSpace(got) == "" {
				t.Fatalf("generated empty name for prompt=%q", tt.prompt)
			}
			if len([]rune(got)) > projectNameMaxLen {
				t.Fatalf("name too long (%d): %q", len([]rune(got)), got)
			}
		})
	}
}

func TestGenerateProjectNameFromPrompt_Fallback(t *testing.T) {
	t.Parallel()

	got := generateProjectNameFromPrompt("，，，。。。!!!")
	if got != "Untitled Project" {
		t.Fatalf("fallback name = %q, want %q", got, "Untitled Project")
	}
}
