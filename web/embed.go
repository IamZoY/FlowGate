package web

import "embed"

// Assets embeds the web/assets directory (index.html, app.js, style.css).
// Use fs.Sub(Assets, "assets") to get a filesystem rooted at the assets dir.
//
//go:embed assets
var Assets embed.FS
