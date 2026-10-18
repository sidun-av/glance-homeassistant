#!/bin/sh
# push.sh — upload a House Control plan.json to a glance-homeassistant server.
#
# Run by launchd (see install.sh) whenever the plan file changes and every
# five minutes; safe to run by hand too. It PUTs the plan to
# <url>/house/plan with the shared secret as a bearer token. Before that
# it asks the server (a plain GET with If-None-Match, which needs no
# secret) whether it already holds these exact bytes, so an unchanged plan
# costs one tiny request, and a plan the server has lost (a recreated data
# volume, a restored backup) is pushed again on the next run.
#
# Settings (environment, all optional):
#   PLAN_FILE  the plan to push
#              (default: ~/Library/Application Support/HouseControl/plan.json)
#   CONF_DIR   holds two files: "url" (the server origin, e.g.
#              http://<lan-host>:<port>) and "secret" (the shared secret,
#              mode 600; leading and trailing whitespace is ignored, as
#              the server does, and inner whitespace is refused)
#              (default: ~/.config/glance-house)
#   STATE_DIR  where the hash of the last successful push is recorded, for
#              reference only (default: ~/Library/Caches/glance-house)
#   LOG_FILE   if set, trimmed to its last 2000 lines once it passes 1 MiB
#
# Exit status: 0 when there was nothing to do, the push worked, or the push
# failed (the next trigger retries); 1 when the configuration is wrong.
# The secret is never printed and never appears on a command line.

set -eu

PLAN_FILE="${PLAN_FILE:-$HOME/Library/Application Support/HouseControl/plan.json}"
CONF_DIR="${CONF_DIR:-$HOME/.config/glance-house}"
STATE_DIR="${STATE_DIR:-$HOME/Library/Caches/glance-house}"
LOG_FILE="${LOG_FILE:-}"

log() {
	printf '%s house-plan-push: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

die() {
	log "$*" >&2
	exit 1
}

# private_file FILE — true when FILE has no group or other permission bits.
private_file() {
	perms=$(ls -lLd -- "$1" | cut -c5-10)
	[ "$perms" = "------" ]
}

sha256() {
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 <"$1" | cut -d' ' -f1
	else
		sha256sum <"$1" | cut -d' ' -f1
	fi
}

trim_log() {
	[ -n "$LOG_FILE" ] && [ -f "$LOG_FILE" ] || return 0
	size=$(wc -c <"$LOG_FILE" | tr -d ' ')
	[ "$size" -gt 1048576 ] || return 0
	tail -n 2000 "$LOG_FILE" >"$work/log" && cat "$work/log" >"$LOG_FILE"
}

[ -f "$CONF_DIR/url" ] || die "missing $CONF_DIR/url (the server origin, e.g. http://<lan-host>:<port>)"
[ -f "$CONF_DIR/secret" ] || die "missing $CONF_DIR/secret"
private_file "$CONF_DIR/secret" || die "refusing: $CONF_DIR/secret is readable by others (chmod 600 it)"

url=$(tr -d ' \t\r\n' <"$CONF_DIR/url")
url=${url%/}
# The secret's surrounding whitespace (a trailing newline, a CR) is dropped,
# exactly as the server trims HOUSE_PLAN_SECRET. Whitespace inside it would
# be kept by the server but cannot be sent safely here, so it is refused.
secret=$(tr '\t\r\n' '   ' <"$CONF_DIR/secret" | sed -e 's/^ *//' -e 's/ *$//')
[ -n "$url" ] || die "$CONF_DIR/url is empty"
[ -n "$secret" ] || die "$CONF_DIR/secret is empty"
case $secret in
*' '*) die "$CONF_DIR/secret contains whitespace; use a secret without spaces (e.g. openssl rand -hex 32)" ;;
esac

if [ ! -f "$PLAN_FILE" ]; then
	log "no plan file at $PLAN_FILE"
	exit 0
fi

mkdir -p "$STATE_DIR"
work=$(mktemp -d "${TMPDIR:-/tmp}/house-plan-push.XXXXXX")
trap 'rm -rf "$work"' EXIT
trim_log

# Hash and upload one snapshot, so a plan rewritten mid-run cannot be
# recorded under the wrong hash.
cp "$PLAN_FILE" "$work/plan.json"
hash=$(sha256 "$work/plan.json")
key="$hash $url"

# The server's ETag is the first 16 hex digits of sha256 of the bytes it
# was given, in quotes: a 304 means it already serves exactly this plan.
# Anything else (200 with another plan, 404 with none, an error, no answer)
# means push.
etag="\"$(printf '%s' "$hash" | cut -c1-16)\""
if probe=$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' \
	-H "If-None-Match: $etag" "$url/house/plan" 2>/dev/null) && [ "$probe" = "304" ]; then
	log "unchanged"
	exit 0
fi

short=$(printf '%s' "$hash" | cut -c1-12)
# The header goes to curl on stdin (printf is a shell builtin), so the
# secret never shows up in ps.
if status=$(printf 'Authorization: Bearer %s\n' "$secret" |
	curl -sS --max-time 20 -X PUT \
		-H @- \
		-H 'Content-Type: application/json' \
		--data-binary @"$work/plan.json" \
		-o "$work/body" -w '%{http_code}' \
		"$url/house/plan"); then
	code=0
else
	code=$?
fi

if [ "$code" -eq 0 ] && [ "$status" -ge 200 ] && [ "$status" -lt 300 ]; then
	printf '%s\n' "$key" >"$STATE_DIR/last.tmp"
	mv "$STATE_DIR/last.tmp" "$STATE_DIR/last"
	log "pushed $short"
	exit 0
fi

# The server's error bodies are short codes such as {"error":"unauthorized"}.
detail=""
if [ -s "$work/body" ]; then
	detail=$(head -c 200 "$work/body" | tr -d '\r\n')
fi
log "push failed (curl $code, HTTP ${status:-000}) $detail"
exit 0
