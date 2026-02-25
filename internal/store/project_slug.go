package store

import (
	"crypto/rand"
	"fmt"
	"strings"
)

const (
	projectSlugMaxLen    = 64
	projectSlugSuffixLen = 8
	projectSlugMaxTries  = 10

	shareSlugLen = 10
)

var projectSlugAlphabet = []byte("abcdefghijklmnopqrstuvwxyz0123456789")

func generateUniqueReadableProjectSlug(
	name string,
	existsFn func(candidate string) (bool, error),
	suffixFn func() (string, error),
) (string, error) {
	if existsFn == nil {
		return "", fmt.Errorf("project slug existsFn is nil")
	}
	if suffixFn == nil {
		suffixFn = randomProjectSlugSuffix
	}

	base := slugifyProjectName(name)
	if base == "" {
		base = "project"
	}

	maxBaseLen := projectSlugMaxLen - 1 - projectSlugSuffixLen
	if maxBaseLen < 1 {
		return "", fmt.Errorf("invalid project slug length config")
	}
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], "-")
		if base == "" {
			base = "project"
		}
	}

	var lastErr error
	for range projectSlugMaxTries {
		suffix, err := suffixFn()
		if err != nil {
			return "", fmt.Errorf("generate project slug suffix: %w", err)
		}
		suffix = strings.ToLower(strings.TrimSpace(suffix))
		if suffix == "" {
			return "", fmt.Errorf("empty project slug suffix")
		}

		candidate := base + "-" + suffix
		if len(candidate) > projectSlugMaxLen {
			candidate = candidate[:projectSlugMaxLen]
		}
		exists, err := existsFn(candidate)
		if err != nil {
			return "", fmt.Errorf("check project slug exists: %w", err)
		}
		if !exists {
			return candidate, nil
		}
		lastErr = ErrConflict
	}

	if lastErr != nil {
		return "", fmt.Errorf("generate unique project slug retries exhausted: %w", lastErr)
	}
	return "", fmt.Errorf("generate unique project slug retries exhausted")
}

func slugifyProjectName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(s))
	lastHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastHyphen = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

func randomProjectSlugSuffix() (string, error) {
	return randomSlugString(projectSlugSuffixLen)
}

func randomShareSlug() (string, error) {
	return randomSlugString(shareSlugLen)
}

func randomSlugString(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("invalid random slug len %d", n)
	}
	buf := make([]byte, n)
	randBuf := make([]byte, n)
	if _, err := rand.Read(randBuf); err != nil {
		return "", err
	}
	for i, v := range randBuf {
		buf[i] = projectSlugAlphabet[int(v)%len(projectSlugAlphabet)]
	}
	return string(buf), nil
}
