#!/bin/sh
# update.sh — vendor House Control's web build at a pinned commit.
#
# Usage: tools/house-dist/update.sh <commit>
#
# Exports web/ and LICENSE of <commit> from a House Control git checkout
# into a fresh temporary directory (git archive: nothing from the
# checkout's working tree, node_modules or local edits leaks in), runs
# `npm ci && npm run build` there, and replaces internal/house/dist/ with
# the result: web/dist without the demo plans/, plus House Control's MIT
# LICENSE and a VERSION file naming the commit. The checkout itself is
# never modified.
#
# The build keeps House Control's own Vite `base: './'`: every asset is
# referenced as ./assets/…, and the widget server serves the page with a
# <base href="<public_url>/house/"> so those resolve under whatever path
# the dashboard reaches it by. The script refuses a build whose
# index.html points anywhere else.
#
# Settings (environment, all optional):
#   HOUSE_CONTROL_SRC     a git checkout that contains <commit>
#                         (default: ../house-control next to this repo)
#   HOUSE_CONTROL_BRANCH  branch name recorded in VERSION
#                         (default: feature/browser-host)
#   HOUSE_DIST_OUT        where to put the build
#                         (default: internal/house/dist in this repo)
#
# Needs git, node and npm. The Docker build does not: the result is
# committed and embedded with go:embed.

set -eu

die() { echo "update.sh: $*" >&2; exit 1; }

[ $# -eq 1 ] && [ -n "$1" ] || die "usage: update.sh <commit>"
commit=$1

repo=$(cd "$(dirname "$0")/../.." && pwd)
src=${HOUSE_CONTROL_SRC:-$repo/../house-control}
branch=${HOUSE_CONTROL_BRANCH:-feature/browser-host}
out=${HOUSE_DIST_OUT:-$repo/internal/house/dist}

[ -d "$src" ] || die "no House Control checkout at $src (set HOUSE_CONTROL_SRC)"
sha=$(git -C "$src" rev-parse --verify --quiet "$commit^{commit}") ||
	die "$commit is not a commit in $src"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/house-dist.XXXXXX")
trap 'rm -rf "$tmp"' EXIT INT TERM

git -C "$src" archive "$sha" web LICENSE | tar -x -C "$tmp"
[ -f "$tmp/LICENSE" ] || die "$sha has no LICENSE"
[ -f "$tmp/web/package-lock.json" ] || die "$sha has no web/package-lock.json"

echo "building house-control $sha"
(
	cd "$tmp/web"
	npm ci --no-audit --no-fund --loglevel=error
	npm run build --silent
) >&2

dist=$tmp/web/dist
[ -f "$dist/index.html" ] || die "the build made no dist/index.html"
rm -rf "$dist/plans"

# Every src=/href= in the page must be a ./assets/ file that exists.
refs=$(grep -oE '(src|href)="[^"]*"' "$dist/index.html" | sed -E 's/^[a-z]+="(.*)"$/\1/') || true
[ -n "$refs" ] || die "index.html references no assets"
for ref in $refs; do
	case $ref in
	./assets/*) [ -f "$dist/${ref#./}" ] || die "index.html references a missing $ref" ;;
	*) die "index.html references $ref, not ./assets/ (is Vite's base still './'?)" ;;
	esac
done

cp "$tmp/LICENSE" "$dist/LICENSE"
printf 'house-control %s\nsource: github.com/sidun-av/house-control (branch %s)\nbuilt: %s\n' \
	"$sha" "$branch" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$dist/VERSION"

mkdir -p "$(dirname "$out")"
rm -rf "$out"
cp -R "$dist" "$out"

echo "vendored house-control $sha into $out ($(du -sk "$out" | cut -f1) KiB)"
