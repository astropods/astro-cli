package chatui

import "embed"

// distFS holds the embedded, prebuilt chat SPA (astro-client's chat experience,
// built via `moon run astro-client:build-chat-embed` and copied into ./webdist
// by the CLI release build). Only a .gitkeep placeholder is tracked in git, so a
// plain `go build` compiles even before the assets are produced; the runtime
// checks for index.html and degrades gracefully when only the placeholder is
// present.
//
// The dir is named "webdist" (not "dist") because the repo-root .gitignore
// ignores every directory named "dist".
//
//go:embed all:webdist
var distFS embed.FS
