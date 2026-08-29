// Package webui embeds the WebUI frontend into the cronctl binary.
package webui

import "embed"

// Frontend is the embedded static UI (index.html, app.js, styles, logos).
//
//go:embed all:frontend
var Frontend embed.FS
