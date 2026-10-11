package web

import "embed"

// DistFS embeds the SPA distribution assets.
//
//go:embed all:dist
var DistFS embed.FS
