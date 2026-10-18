#!/bin/sh
# uninstall.sh — stop and remove the launchd job installed by install.sh.
#
#   uninstall.sh            remove the LaunchAgent
#   uninstall.sh --daemon   remove the LaunchDaemon (needs sudo)
#   uninstall.sh --dry-run  print what would be done (combine with --daemon)
#
# Keeps ~/.config/glance-house (url, secret) and the log; forgets the last
# pushed hash, so a later install pushes the plan again.
#
# Environment (optional): STATE_DIR as in push.sh; LAUNCHCTL and SUDO to
# substitute those commands.

set -eu

LABEL=com.glance-homeassistant.house-plan-push
STATE_DIR="${STATE_DIR:-$HOME/Library/Caches/glance-house}"
LAUNCHCTL="${LAUNCHCTL:-/bin/launchctl}"
SUDO="${SUDO-sudo}"
APP_DIR="$HOME/Library/Application Support/glance-house"
AGENT_PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DAEMON_PLIST="/Library/LaunchDaemons/$LABEL.plist"

daemon=0
dry=0
for arg in "$@"; do
	case "$arg" in
	--daemon) daemon=1 ;;
	--dry-run) dry=1 ;;
	-h | --help)
		sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "uninstall.sh: unknown argument: $arg" >&2
		exit 2
		;;
	esac
done

run() {
	if [ "$dry" -eq 1 ]; then
		printf 'would run: %s\n' "$*"
	else
		"$@"
	fi
}

if [ "$daemon" -eq 1 ]; then
	run $SUDO "$LAUNCHCTL" bootout "system/$LABEL" 2>/dev/null || true
	run $SUDO rm -f "$DAEMON_PLIST"
	other="$AGENT_PLIST"
	echo "removed LaunchDaemon $LABEL"
else
	run "$LAUNCHCTL" bootout "gui/$(id -u)/$LABEL" 2>/dev/null || true
	run rm -f "$AGENT_PLIST"
	other="$DAEMON_PLIST"
	echo "removed LaunchAgent $LABEL"
fi

# The installed push.sh is shared by both kinds of job; keep it while the
# other one is still installed.
if [ ! -f "$other" ]; then
	run rm -f "$APP_DIR/push.sh"
	if [ "$dry" -eq 0 ]; then
		rmdir "$APP_DIR" 2>/dev/null || true
	fi
fi
run rm -f "$STATE_DIR/last"
