package house

import (
	"embed"
	"io/fs"
)

// dist is House Control's web build (index.html, assets/, LICENSE and a
// VERSION naming the commit), vendored by tools/house-dist/update.sh.
//
//go:embed all:dist
var dist embed.FS

// DistFS is the vendored House Control build, rooted at its index.html:
// what Handler.Dist serves the 3D page from.
func DistFS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path; fs.Sub cannot fail on it
	}
	return sub
}
