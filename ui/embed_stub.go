//go:build !embedui

// Package ui provides an empty stand-in for the embedded web UI when built
// without `-tags embedui` (e.g. plain `go build ./...` during development,
// when ui/dist/ may not exist yet). See embed.go for the release variant.
package ui

import "embed"

// Dist is empty in non-embedui builds. Callers should check Embedded
// before serving from it -- e.g. falling back to proxying the Vite dev
// server, or a "run `make ui`" message.
var Dist embed.FS

// Embedded reports whether the web UI is compiled into this binary.
const Embedded = false
