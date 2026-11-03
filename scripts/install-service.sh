#!/usr/bin/env bash
#
# Install the fme scheduler as a systemd user service. The service runs
# `fme -action=schedule` with Restart=on-failure, so a crashed or logged-out
# cycle comes back automatically.
#
# Usage: scripts/install-service.sh [--dry-run] [--uninstall]
#
# The unit is written to ${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/.
# A user service starts with your login session, which is what gives Chrome
# access to the OS keyring to decrypt the copied Facebook cookies. Do not
# enable lingering unless you also handle keyring unlock at boot.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

UNIT_NAME="fme-schedule.service"
UNIT_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
UNIT_PATH="$UNIT_DIR/$UNIT_NAME"
BINARY="$ROOT_DIR/fme"
LOG_FILE="$ROOT_DIR/data/scheduler.log"

DRY_RUN=0
UNINSTALL=0

usage() {
    cat <<'EOF'
Install the fme scheduler as a systemd user service.

Usage: scripts/install-service.sh [options]

Options:
  --dry-run    Print the unit and exit; change nothing
  --uninstall  Stop, disable, and remove the unit
  -h, --help   Show this help

The service runs: fme -action=schedule -log-file=data/scheduler.log
Edit search-terms.txt and restart the service to change the queries.
EOF
    exit "${1:-0}"
}

info() { printf '    %s\n' "$*"; }
ok() { printf '    \033[32m✓\033[0m %s\n' "$*"; }
die() { printf '    \033[31m✗\033[0m %s\n' "$*" >&2; exit 1; }

while [ "$#" -gt 0 ]; do
    case "$1" in
        --dry-run) DRY_RUN=1; shift ;;
        --uninstall) UNINSTALL=1; shift ;;
        -h|--help) usage 0 ;;
        *) printf 'unknown option: %s\n' "$1" >&2; usage 1 ;;
    esac
done

unit_contents() {
    cat <<EOF
[Unit]
Description=FME Marketplace scraper scheduler
After=network-online.target

[Service]
Type=simple
WorkingDirectory=$ROOT_DIR
ExecStart=$BINARY -action=schedule -log-file=$LOG_FILE
Restart=on-failure
RestartSec=60
TimeoutStopSec=90

[Install]
WantedBy=default.target
EOF
}

uninstall() {
    systemctl --user disable --now "$UNIT_NAME" 2>/dev/null || true
    rm -f "$UNIT_PATH"
    systemctl --user daemon-reload
    ok "removed $UNIT_NAME"
}

install() {
    [ -x "$BINARY" ] || die "missing $BINARY; run 'just build' first"
    [ -f "$ROOT_DIR/search-terms.txt" ] || die "missing $ROOT_DIR/search-terms.txt"

    if [ "$DRY_RUN" -eq 1 ]; then
        info "would write $UNIT_PATH:"
        unit_contents
        return 0
    fi

    mkdir -p "$UNIT_DIR"
    unit_contents > "$UNIT_PATH"
    ok "wrote $UNIT_PATH"

    systemctl --user daemon-reload
    systemctl --user enable --now "$UNIT_NAME"
    ok "started $UNIT_NAME"
    info "state:  $ROOT_DIR/data/scheduler-state.json"
    info "log:    $LOG_FILE"
    info "status: just service-status"
    info "logs:   just service-logs"
}

if [ "$UNINSTALL" -eq 1 ]; then
    uninstall
else
    install
fi
