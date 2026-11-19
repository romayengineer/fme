#!/usr/bin/env bash
#
# Sync the scraped data/ directory with a remote host over SSH/rsync.
#
# Defaults to a push (local -> remote) that MERGES: files only present on the
# remote are never removed. Use --pull to reverse the direction or --delete to
# mirror the source exactly (destructive).
#
# Usage:
#   just sync [--pull] [--delete] [--dry-run] [--save]
#             [--host <user@host>] [--dir <remote-project-dir>]
#
# Configuration (environment variables, all optional):
#   FME_SYNC_REMOTE       SSH target of the server (default: maxi@192.168.0.235)
#   FME_SYNC_REMOTE_DIR   Project dir on the server, relative to its $HOME
#                         (default: Projects/romayengineer/fme)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

PULL=0
DELETE=0
DRY_RUN=0
RUN_SAVE=0
HOST_OVERRIDE=""
DIR_OVERRIDE=""

usage() {
    cat <<'EOF'
Sync the scraped data/ directory with a remote host over SSH/rsync.

Defaults to a push (local -> remote) that MERGES: files only present on the
remote are never removed. Use --pull to reverse the direction or --delete to
mirror the source exactly (destructive).

Usage:
  just sync [--pull] [--delete] [--dry-run] [--save]
            [--host <user@host>] [--dir <remote-project-dir>]

Options:
  --pull                Pull data/ from the remote instead of pushing
  --delete              Delete destination files missing from the source (mirror)
  --dry-run, -n         Show what would transfer without changing anything
  --save                After a push, run `just save` on the remote (imports
                        data/__Details into PocketBase)
  --host <user@host>    SSH target (overrides FME_SYNC_REMOTE)
  --dir <path>          Project dir on the remote, relative to its $HOME
                        (overrides FME_SYNC_REMOTE_DIR)
  -h, --help            Show this help

Configuration (environment variables, all optional):
  FME_SYNC_REMOTE       SSH target of the server (default: maxi@192.168.0.235)
  FME_SYNC_REMOTE_DIR   Project dir on the server, relative to its $HOME
                        (default: Projects/romayengineer/fme)
EOF
    exit "${1:-0}"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --pull) PULL=1 ;;
        --delete) DELETE=1 ;;
        --dry-run | -n) DRY_RUN=1 ;;
        --save) RUN_SAVE=1 ;;
        --host)
            shift
            HOST_OVERRIDE="${1:-}"
            ;;
        --dir)
            shift
            DIR_OVERRIDE="${1:-}"
            ;;
        -h | --help) usage 0 ;;
        *)
            echo "unknown option: $1" >&2
            usage 1
            ;;
    esac
    shift
done

step() { printf '\n\033[1;34m==>\033[0m \033[1m%s\033[0m\n' "$*"; }
info() { printf '    %s\n' "$*"; }
ok() { printf '    \033[32m✓\033[0m %s\n' "$*"; }
warn() { printf '    \033[33m!\033[0m %s\n' "$*"; }
die() {
    printf '\n\033[31merror:\033[0m %s\n' "$*" >&2
    exit 1
}

cd "$ROOT_DIR" || exit 1

# --- configuration -----------------------------------------------------------
# Precedence: CLI flag > environment variable > .env key > built-in default.

env_value() {
    [ -f "$ROOT_DIR/.env" ] || return 0
    sed -n "s/^$1=//p" "$ROOT_DIR/.env" | tail -1
}

REMOTE="${HOST_OVERRIDE:-${FME_SYNC_REMOTE:-$(env_value FME_SYNC_REMOTE)}}"
REMOTE="${REMOTE:-maxi@192.168.0.235}"

REMOTE_DIR="${DIR_OVERRIDE:-${FME_SYNC_REMOTE_DIR:-$(env_value FME_SYNC_REMOTE_DIR)}}"
REMOTE_DIR="${REMOTE_DIR:-Projects/romayengineer/fme}"

# Relative remote paths resolve against the remote $HOME; strip a leading ./
REMOTE_DIR="${REMOTE_DIR#./}"
REMOTE_DIR="${REMOTE_DIR%/}"

if [ "$PULL" -eq 1 ] && [ "$RUN_SAVE" -eq 1 ]; then
    die "--save only applies to a push; drop --pull or --save"
fi

printf '\033[1mfacebook marketplace extractor — data sync\033[0m\n'
info "repo:        $ROOT_DIR"
info "remote:      $REMOTE:$REMOTE_DIR"
info "direction:   $([ "$PULL" -eq 1 ] && echo 'pull (remote -> local)' || echo 'push (local -> remote)')"
info "mode:        $([ "$DELETE" -eq 1 ] && echo 'mirror (--delete)' || echo 'merge (keep destination extras)')"
[ "$DRY_RUN" -eq 1 ] && info "dry run:     yes (nothing will change)"

# --- prerequisites -----------------------------------------------------------

step "Checking prerequisites"

command -v rsync >/dev/null 2>&1 || die "rsync is not installed"
command -v ssh >/dev/null 2>&1 || die "ssh is not installed"
ok "rsync and ssh present"

if [ ! -d "$ROOT_DIR/data" ]; then
    if [ "$PULL" -eq 1 ]; then
        warn "local data/ does not exist; it will be created by the pull"
    else
        die "local data/ does not exist; nothing to push"
    fi
fi

step "Checking the remote"

ssh -o BatchMode=yes -o ConnectTimeout=8 "$REMOTE" true 2>/dev/null ||
    die "cannot reach $REMOTE over SSH (try: ssh $REMOTE)"
ok "SSH connection to $REMOTE works"

ssh -o BatchMode=yes -o ConnectTimeout=8 "$REMOTE" "test -d \"\$HOME/$REMOTE_DIR/.git\"" 2>/dev/null ||
    die "no git project at $REMOTE:~/$REMOTE_DIR (clone it first, or pass --dir)"
ok "remote project found at ~/$REMOTE_DIR"

# --- transfer ----------------------------------------------------------------

step "Syncing data/"

RSYNC_ARGS=(-av --exclude='.DS_Store')
[ "$DELETE" -eq 1 ] && RSYNC_ARGS+=(--delete)
[ "$DRY_RUN" -eq 1 ] && RSYNC_ARGS+=(--dry-run)

if [ "$PULL" -eq 1 ]; then
    rsync "${RSYNC_ARGS[@]}" "$REMOTE:$REMOTE_DIR/data/" "$ROOT_DIR/data/"
else
    rsync "${RSYNC_ARGS[@]}" "$ROOT_DIR/data/" "$REMOTE:$REMOTE_DIR/data/"
fi

if [ "$DRY_RUN" -eq 1 ]; then
    ok "dry run complete (no changes made)"
else
    ok "data/ synced"
fi

# --- optional remote import --------------------------------------------------

if [ "$RUN_SAVE" -eq 1 ] && [ "$DRY_RUN" -eq 0 ]; then
    step "Importing into PocketBase on the remote"
    ssh "$REMOTE" "cd \"\$HOME/$REMOTE_DIR\" && just save"
    ok "remote PocketBase updated"
fi

# --- done --------------------------------------------------------------------

step "Sync complete"
if [ "$PULL" -eq 1 ]; then
    info "details now local: $(find "$ROOT_DIR/data/__Details" -name 'detail_*.json' 2>/dev/null | wc -l | tr -d ' ')"
else
    info "verify on the remote with: ssh $REMOTE 'find ~/$REMOTE_DIR/data/__Details -name \"detail_*.json\" | wc -l'"
fi
