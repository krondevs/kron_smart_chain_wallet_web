package webui

import "embed"

// Files contains the production React build. Vite writes to
// webwallet/internal/webui/dist. The placeholder is replaced by `make web`.
//
//go:embed all:dist
var Files embed.FS
