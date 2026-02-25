package web

import "embed"

// FS contains HTML templates and static assets for server rendering.
//
//go:embed templates/*.tmpl static/*
var FS embed.FS
