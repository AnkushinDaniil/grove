//go:build embedui

// Package ui embeds the built web UI (dist/) into the daemon binary for
// release builds. Requires `npm run build` (see the Makefile's `ui` target)
// to have produced ui/dist/ before compiling with `-tags embedui` --
// dist/ is gitignored and generated, never committed.
package ui

import "embed"

//go:embed all:dist
var Dist embed.FS

// Embedded reports whether the web UI is compiled into this binary.
const Embedded = true
