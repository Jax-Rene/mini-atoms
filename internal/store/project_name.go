package store

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const projectNameMaxLen = 36

var chinesePromptPrefixes = []string{
	"请帮我做一个",
	"帮我做一个",
	"请帮我做个",
	"帮我做个",
	"我想做一个",
	"我想做个",
	"帮我搭一个",
	"帮我生成一个",
	"创建一个",
	"做一个",
	"做个",
}

var englishPromptPrefixes = []string{
	"please build a ",
	"please build an ",
	"please create a ",
	"please create an ",
	"build a ",
	"build an ",
	"create a ",
	"create an ",
	"make a ",
	"make an ",
}

func generateProjectNameFromPrompt(prompt string) string {
	s := normalizePromptText(prompt)
	if s == "" {
		return "Untitled Project"
	}

	s = firstPromptClause(s)
	s = trimPromptPrefix(s)
	s = strings.TrimSpace(trimPunctuation(s))
	if s == "" {
		return "Untitled Project"
	}

	s = collapseSpaces(s)
	s = trimRunes(s, projectNameMaxLen)
	s = strings.TrimSpace(trimPunctuation(s))
	if s == "" {
		return "Untitled Project"
	}

	return s
}

func normalizePromptText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return collapseSpaces(s)
}

func firstPromptClause(s string) string {
	if s == "" {
		return s
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		switch r {
		case '\n', '。', '，', ',', '！', '!', '？', '?', ';', '；', ':', '：':
			return true
		default:
			return false
		}
	})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			return p
		}
	}
	return s
}

func trimPromptPrefix(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range chinesePromptPrefixes {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}

	lower := strings.ToLower(s)
	for _, prefix := range englishPromptPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}

	return s
}

func trimPunctuation(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		if unicode.IsSpace(r) {
			return true
		}
		return unicode.IsPunct(r) || strings.ContainsRune("，。！？；：、【】（）《》“”‘’", r)
	})
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func trimRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	rs := []rune(s)
	return string(rs[:max])
}
