package editor

import _ "embed"

// pageHTML is the whole editor: one page, no framework. {{BASE}} is the
// path prefix the page was served under (e.g. "/ha-widget"), so its
// fetches work through a prefix-keeping reverse proxy too.
//
//go:embed page.html
var pageHTML string
