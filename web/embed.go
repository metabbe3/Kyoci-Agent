// Package web embeds the production build of the dashboard SPA. The build
// output lands in web/dist/ via `npm run build` (Vite). At dev time the SPA
// runs on Vite's own :5173 dev server; this embedded FS only matters for
// production single-binary deploys.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
