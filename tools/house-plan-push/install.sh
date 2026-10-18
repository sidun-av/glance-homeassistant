#!/bin/sh
# install.sh — run push.sh from launchd: when the plan changes, every five
# minutes, and at login. Idempotent; run it again after an update.
#
#   install.sh             a LaunchAgent for the current user (no sudo)
#   install.sh --daemon    the same job as a LaunchDaemon that runs as the
#                          current user (needs sudo). Use it when macOS Local
#                          Network privacy blocks the agent: the log then
#                          shows "push failed (curl 7, ...)".
#   install.sh --dry-run   print what would be done and the rendered plist,
#                          change nothing (combine with --daemon)
#
# Before installing, create the two config files (they are never created or
# overwritten here):
#   ~/.config/glance-house/url     the server origin, e.g. http://<lan-host>:<port>
#   ~/.config/glance-house/secret  the shared secret, mode 600
#
# Environment (optional): PLAN_FILE, CONF_DIR, STATE_DIR as in push.sh;
# LAUNCHCTL and SUDO to substitute those commands.

set -eu

LABEL=com.glance-homeassistant.house-plan-push
PLAN_FILE="${PLAN_FILE:-$HOME/Library/Application Support/HouseControl/plan.json}"
CONF_DIR="${CONF_DIR:-$HOME/.config/glance-house}"
STATE_DIR="${STATE_DIR:-$HOME/Library/Caches/glance-house}"
LAUNCHCTL="${LAUNCHCTL:-/bin/launchctl}"
SUDO="${SUDO-sudo}"
APP_DIR="$HOME/Library/Application Support/glance-house"
PUSH_SH="$APP_DIR/push.sh"
LOG_FILE="$HOME/Library/Logs/glance-house-push.log"
AGENT_PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DAEMON_PLIST="/Library/LaunchDaemons/$LABEL.plist"

daemon=0
dry=0
for arg in "$@"; do
	case "$arg" in
	--daemon) daemon=1 ;;
	--dry-run) dry=1 ;;
	-h | --help)
		sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "install.sh: unknown argument: $arg" >&2
		exit 2
		;;
	esac
done

here=$(cd "$(dirname "$0")" && pwd)
uid=$(id -u)
user=$(id -un)

say() { printf '%s\n' "$*"; }
die() {
	printf 'install.sh: %s\n' "$*" >&2
	exit 1
}

# run CMD... — run it, or only print it in --dry-run.
run() {
	if [ "$dry" -eq 1 ]; then
		say "would run: $*"
	else
		"$@"
	fi
}

xml_escape() { printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g'; }
# value for the right-hand side of a sed s||| expression
sed_value() { xml_escape "$1" | sed -e 's/[\\|&]/\\&/g'; }

render() {
	if [ "$daemon" -eq 1 ]; then
		username="<key>UserName</key><string>$(sed_value "$user")</string>"
	else
		username=""
	fi
	sed -e "s|@LABEL@|$(sed_value "$LABEL")|g" \
		-e "s|@USERNAME@|$username|g" \
		-e "s|@PUSH_SH@|$(sed_value "$PUSH_SH")|g" \
		-e "s|@HOME@|$(sed_value "$HOME")|g" \
		-e "s|@PLAN_FILE@|$(sed_value "$PLAN_FILE")|g" \
		-e "s|@CONF_DIR@|$(sed_value "$CONF_DIR")|g" \
		-e "s|@STATE_DIR@|$(sed_value "$STATE_DIR")|g" \
		-e "s|@LOG_FILE@|$(sed_value "$LOG_FILE")|g" \
		-e '/^[[:space:]]*$/d' \
		"$here/$LABEL.plist.in"
}

# bootstrap DOMAIN PLIST [sudo] — a just-booted-out job can take a moment
# to go away, so retry briefly.
bootstrap() {
	n=0
	while :; do
		if [ "$dry" -eq 1 ]; then
			say "would run: ${3:+$3 }$LAUNCHCTL bootstrap $1 $2"
			return 0
		fi
		if ${3:-} "$LAUNCHCTL" bootstrap "$1" "$2"; then
			return 0
		fi
		n=$((n + 1))
		[ "$n" -lt 3 ] || die "launchctl bootstrap $1 failed; see 'launchctl print $1/$LABEL'"
		sleep 1
	done
}

[ "$(uname -s)" = Darwin ] || [ "$LAUNCHCTL" != /bin/launchctl ] || die "this installs a macOS launchd job"
[ -f "$here/push.sh" ] || die "push.sh not found next to install.sh"

if [ "$dry" -eq 0 ]; then
	mkdir -p "$CONF_DIR"
	chmod 700 "$CONF_DIR"
fi
[ -s "$CONF_DIR/url" ] || die "create $CONF_DIR/url first (the server origin, e.g. http://<lan-host>:<port>)"
[ -s "$CONF_DIR/secret" ] || die "create $CONF_DIR/secret first (e.g. openssl rand -hex 32), mode 600"
[ "$(ls -lLd -- "$CONF_DIR/secret" | cut -c5-10)" = "------" ] ||
	die "$CONF_DIR/secret is readable by others; chmod 600 it"

plist_tmp=$(mktemp "${TMPDIR:-/tmp}/house-plan-push.XXXXXX")
trap 'rm -f "$plist_tmp"' EXIT
render >"$plist_tmp"
if command -v plutil >/dev/null 2>&1; then
	plutil -lint -s "$plist_tmp" || die "rendered plist is invalid"
fi

if [ "$dry" -eq 1 ]; then
	say "would install $here/push.sh -> $PUSH_SH"
	if [ "$daemon" -eq 1 ]; then
		say "would write $DAEMON_PLIST:"
	else
		say "would write $AGENT_PLIST:"
	fi
	cat "$plist_tmp"
fi

if [ "$dry" -eq 0 ]; then
	mkdir -p "$APP_DIR" "$HOME/Library/Logs" "$STATE_DIR"
	cp "$here/push.sh" "$PUSH_SH.tmp"
	chmod 755 "$PUSH_SH.tmp"
	mv "$PUSH_SH.tmp" "$PUSH_SH"
	# Created by the user, so push.sh can trim it whichever way launchd runs.
	touch "$LOG_FILE"
fi

if [ "$daemon" -eq 1 ]; then
	[ -f "$AGENT_PLIST" ] && say "note: a LaunchAgent is installed too; remove it with: $here/uninstall.sh"
	run $SUDO "$LAUNCHCTL" bootout "system/$LABEL" 2>/dev/null || true
	run $SUDO install -m 644 -o root -g wheel "$plist_tmp" "$DAEMON_PLIST"
	bootstrap system "$DAEMON_PLIST" "$SUDO"
	say "installed LaunchDaemon $LABEL (runs as $user)"
else
	[ -f "$DAEMON_PLIST" ] && say "note: a LaunchDaemon is installed too; remove it with: $here/uninstall.sh --daemon"
	run "$LAUNCHCTL" bootout "gui/$uid/$LABEL" 2>/dev/null || true
	if [ "$dry" -eq 0 ]; then
		mkdir -p "$(dirname "$AGENT_PLIST")"
		cp "$plist_tmp" "$AGENT_PLIST"
		chmod 644 "$AGENT_PLIST"
	fi
	bootstrap "gui/$uid" "$AGENT_PLIST"
	say "installed LaunchAgent $LABEL"
fi
[ "$dry" -eq 1 ] || say "log: $LOG_FILE"
