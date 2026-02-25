package store

import (
	"errors"
	"strings"
	"testing"
)

func TestGenerateUniqueReadableProjectSlug_RetriesOnCollision(t *testing.T) {
	t.Parallel()

	suffixes := []string{"dup12345", "uniq6789"}
	var suffixCalls int

	slug, err := generateUniqueReadableProjectSlug(
		"My Resume Builder",
		func(candidate string) (bool, error) {
			return candidate == "my-resume-builder-dup12345", nil
		},
		func() (string, error) {
			if suffixCalls >= len(suffixes) {
				return "", errors.New("unexpected extra suffix request")
			}
			s := suffixes[suffixCalls]
			suffixCalls++
			return s, nil
		},
	)
	if err != nil {
		t.Fatalf("generate slug: %v", err)
	}

	if slug != "my-resume-builder-uniq6789" {
		t.Fatalf("slug = %q", slug)
	}
	if suffixCalls != 2 {
		t.Fatalf("suffix calls = %d, want 2", suffixCalls)
	}
}

func TestGenerateUniqueReadableProjectSlug_FallbackAndLength(t *testing.T) {
	t.Parallel()

	slug, err := generateUniqueReadableProjectSlug(
		"我的第一个项目🚀",
		func(candidate string) (bool, error) { return false, nil },
		func() (string, error) { return "abc12345", nil },
	)
	if err != nil {
		t.Fatalf("generate slug: %v", err)
	}

	if slug != "project-abc12345" {
		t.Fatalf("slug = %q, want %q", slug, "project-abc12345")
	}

	longName := strings.Repeat("very-long-project-name-", 10)
	slug2, err := generateUniqueReadableProjectSlug(
		longName,
		func(candidate string) (bool, error) { return false, nil },
		func() (string, error) { return "z9y8x7w6", nil },
	)
	if err != nil {
		t.Fatalf("generate long slug: %v", err)
	}
	if len(slug2) > projectSlugMaxLen {
		t.Fatalf("len(slug2) = %d, want <= %d; slug=%q", len(slug2), projectSlugMaxLen, slug2)
	}
	if !strings.HasSuffix(slug2, "-z9y8x7w6") {
		t.Fatalf("slug2 suffix mismatch: %q", slug2)
	}
}
