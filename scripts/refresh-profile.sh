#!/usr/bin/env bash
#
# Re-copy a logged-in Chrome profile into the scraper's user-data dir, so an expired
# Facebook session is replaced by the session you already have in everyday Chrome.
#
# Usage: just refresh-profile [--list] [--auto] [--if-expired] [--dry-run] [...]
#
# Destination and profile come from .env (FME_USER_DATA_DIR / FME_PROFILE_DIRECTORY),
# so the copy always lands where the scraper looks. Environment variables override
# .env; command-line flags override everything.
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

# shellcheck source=lib/chrome-profile.sh
. "$SCRIPT_DIR/lib/chrome-profile.sh"

ENV_FILE="$ROOT_DIR/.env"

info() { [ "${QUIET:-0}" -eq 0 ] && printf '    %s\n' "$*"; return 0; }
ok() { [ "${QUIET:-0}" -eq 0 ] && printf '    \033[32m✓\033[0m %s\n' "$*"; return 0; }
warn() { printf '    \033[33m!\033[0m %s\n' "$*"; }
step() { [ "${QUIET:-0}" -eq 0 ] && printf '\n\033[1;34m==>\033[0m \033[1m%s\033[0m\n' "$*"; return 0; }
die() {
    printf '\n\033[31merror:\033[0m %s\n' "$*" >&2
    exit 1
}

usage() {
    cat <<'EOF'
Re-copy a logged-in Chrome profile into the scraper's user-data dir.

Usage: just refresh-profile [options]

Options:
  --list              List Chrome profiles that hold a usable Facebook session
  --auto              Copy the profile with the freshest (persistent) session
  --profile NAME      Chrome profile to copy (default: Default, or .env value)
  --src DIR           Chrome user-data dir to copy from
  --dest DIR          Destination user-data dir (default: FME_USER_DATA_DIR)
  --if-expired        No-op when the destination session is still valid
  --dry-run           Print what would happen without copying
  --force             Copy even if the source session cannot be verified
  --no-env            Do not update .env (used by `just setup`)
  --quiet             Only print warnings and errors
  -h, --help          Show this help

The destination and profile default to .env (FME_USER_DATA_DIR and
FME_PROFILE_DIRECTORY); environment variables FME_CHROME_SRC / FME_PROFILE_NAME /
FME_CHROME_DIR override those. The copy is staged and verified before it replaces
the existing profile, so a failed copy never leaves the scraper without a session.

Exit codes: 0 copied (or nothing to do), 1 error.
EOF
}

SRC=""
DEST=""
PROFILE=""
LIST=0
AUTO=0
IF_EXPIRED=0
DRY_RUN=0
FORCE=0
NO_ENV=0
QUIET=0

while [ $# -gt 0 ]; do
    case "$1" in
        --list) LIST=1 ;;
        --auto) AUTO=1 ;;
        --if-expired) IF_EXPIRED=1 ;;
        --dry-run) DRY_RUN=1 ;;
        --force) FORCE=1 ;;
        --no-env) NO_ENV=1 ;;
        --quiet | -q) QUIET=1 ;;
        --profile=*) PROFILE="${1#*=}" ;;
        --profile) PROFILE="${2:-}"; shift ;;
        --src=*) SRC="${1#*=}" ;;
        --src) SRC="${2:-}"; shift ;;
        --dest=*) DEST="${1#*=}" ;;
        --dest) DEST="${2:-}"; shift ;;
        -h | --help) usage; exit 0 ;;
        *) usage >&2; die "unknown option: $1" ;;
    esac
    shift
done

# --- resolve configuration ---------------------------------------------------

env_dest="$(fme_env_get "$ENV_FILE" FME_USER_DATA_DIR)"
env_profile="$(fme_env_get "$ENV_FILE" FME_PROFILE_DIRECTORY)"

DEST="${DEST:-${FME_USER_DATA_DIR:-$env_dest}}"
DEST="${DEST:-$HOME/.local/share/fme-chrome}"
SRC="${SRC:-${FME_CHROME_SRC:-$(fme_default_chrome_src)}}"
PROFILE="${PROFILE:-${FME_PROFILE_NAME:-${env_profile:-Default}}}"

[ -n "$DEST" ] || die "no destination: set FME_USER_DATA_DIR in .env or pass --dest"
[ -n "$SRC" ] || die "no Chrome user-data dir: pass --src"

if [ "$LIST" -eq 1 ]; then
    printf 'Chrome user-data dir: %s\n' "$SRC"
    if [ ! -d "$SRC" ]; then
        die "Chrome user-data dir not found: $SRC"
    fi
    printf 'Profiles with a usable Facebook session:\n'
    if ! list_fb_profiles "$SRC"; then
        printf '        (none)\n'
        printf '\nOpen Chrome, sign in at https://www.facebook.com, then retry.\n'
    fi
    exit 0
fi

[ -d "$SRC" ] || die "Chrome user-data dir not found: $SRC (use --src)"

if [ "$IF_EXPIRED" -eq 1 ] && [ ! -f "$ENV_FILE" ]; then
    warn ".env not found; skipping profile refresh (run 'just setup')"
    exit 0
fi

if [ "$AUTO" -eq 1 ]; then
    PROFILE="$(best_fb_profile "$SRC" || true)"
    [ -n "$PROFILE" ] || die "no Chrome profile in $SRC has a usable Facebook session (see --list)"
fi

current="${env_profile:-Default}"

# --- fast path: nothing to do ------------------------------------------------

if [ "$IF_EXPIRED" -eq 1 ] && fb_session_status "$DEST/$current"; then
    info "Facebook session in '$current' is valid until $(fme_epoch_to_date "$FB_EXPIRY"); no refresh needed"
    exit 0
fi

src_profile="$SRC/$PROFILE"
[ -d "$src_profile" ] || die "profile '$PROFILE' not found in $SRC (try: just refresh-profile --list)"

if fb_session_status "$src_profile"; then
    src_expiry="$FB_EXPIRY"
elif [ "$FORCE" -eq 1 ]; then
    src_expiry=0
    warn "profile '$PROFILE' has no verifiable Facebook session; copying anyway (--force)"
else
    printf '\n\033[31merror:\033[0m profile '\''%s'\'' in %s has no usable Facebook session.\n\n' \
        "$PROFILE" "$SRC" >&2
    printf 'Log into Facebook once in Chrome, then retry:\n' >&2
    printf '  1. open Chrome and sign in at https://www.facebook.com\n' >&2
    printf '  2. just refresh-profile\n\n' >&2
    printf 'Profiles with a usable session found:\n' >&2
    if ! list_fb_profiles "$SRC" >&2; then
        printf '        (none)\n' >&2
    fi
    exit 1
fi

# --- plan --------------------------------------------------------------------

if profile_in_use "$DEST"; then
    die "$DEST is in use by a running Chrome; close that browser (or stop the scraper) and retry"
fi

if [ "$DRY_RUN" -eq 1 ]; then
    step "Dry run"
    info "source:      $src_profile"
    info "destination: $DEST/$PROFILE"
    info "source session expires: $(fme_epoch_to_date "$src_expiry")"
    if [ "$(fme_env_get "$ENV_FILE" FME_PROFILE_DIRECTORY)" != "$PROFILE" ]; then
        info "would update .env: FME_PROFILE_DIRECTORY=$PROFILE"
    fi
    exit 0
fi

# --- copy --------------------------------------------------------------------

step "Refreshing the Facebook session"
info "source: $src_profile"
info "dest:   $DEST/$PROFILE"
if pgrep -f '/opt/google/chrome/chrome|Google Chrome.app|google-chrome' >/dev/null 2>&1; then
    info "Chrome is running; copying live (cookies are read through the WAL)"
fi

copy_local_state "$SRC" "$DEST"
verify=1
if [ "$FORCE" -eq 1 ]; then
    verify=0
fi
if ! copy_profile "$src_profile" "$DEST" "$PROFILE" "$verify"; then
    die "copy failed or the copied profile has no usable Facebook session; quit Chrome and retry"
fi

printf 'profile=%s\nsource=%s\nrefreshed_at=%s\n' \
    "$PROFILE" "$SRC" "$(date -u +%FT%TZ)" >"$DEST/.fme-setup"

if fb_session_status "$DEST/$PROFILE"; then
    ok "copied to $DEST/$PROFILE"
    ok "Facebook session valid until $(fme_epoch_to_date "$FB_EXPIRY")"
else
    warn "copied to $DEST/$PROFILE, but no usable Facebook session was found"
    exit 1
fi

# --- keep .env in sync -------------------------------------------------------

if [ "$NO_ENV" -eq 1 ]; then
    :
elif [ -f "$ENV_FILE" ]; then
    if [ "$env_dest" != "$DEST" ] || [ "$env_profile" != "$PROFILE" ]; then
        cp -f "$ENV_FILE" "$ENV_FILE.bak"
        fme_env_set "$ENV_FILE" FME_USER_DATA_DIR "$DEST"
        fme_env_set "$ENV_FILE" FME_PROFILE_DIRECTORY "$PROFILE"
        info "updated .env (backup: .env.bak)"
    fi
else
    warn ".env not found; run 'just setup' once to create it"
fi

step "Done"
info "verify with:  just doctor"
info 'collect data: just search "macbook m1"'
