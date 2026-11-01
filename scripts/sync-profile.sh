#!/usr/bin/env bash
#
# Copy a Chrome profile into the profile the scraper drives. Run it manually
# whenever the copied session expires, or let `just` run it automatically before
# every scrape (`--auto`).
#
# The source is the profile named by --profile / FME_PROFILE_NAME, and `Default`
# when neither is set. The copy happens even when the source is logged out; it
# only warns. The destination always stays the profile .env points at, so a
# refresh never invalidates .env. `--detect` opts into picking the Chrome
# profile that actually holds a Facebook login instead.
#
# Usage: just sync-profile [--dry-run] [--profile NAME]
#                          [--source DIR] [--dest DIR] [--detect]
#        just sync-profile --auto        # quiet, skip when already fresh
#
# Configuration (environment variables, all optional):
#   FME_CHROME_SRC       Chrome user-data dir to copy from
#   FME_PROFILE_NAME     Chrome profile name to copy (default: Default)
#   FME_CHROME_DIR       Destination user-data dir (default: .env / fme-chrome)
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

IF_NEEDED=0
QUIET=0
AUTO=0
DRY_RUN=0
DETECT=0
CLI_SOURCE=""
CLI_PROFILE=""
CLI_DEST=""

usage() {
    cat <<'EOF'
Copy a Chrome profile into the profile the scraper drives.

Usage: just sync-profile [options]

Options:
  --profile NAME   Chrome profile to copy (default: FME_PROFILE_NAME, else Default)
  --detect         Copy whichever Chrome profile currently has a Facebook login
  --source DIR     Chrome user-data dir (default: FME_CHROME_SRC or the OS path)
  --dest DIR       Destination user-data dir (default: .env FME_USER_DATA_DIR)
  --if-needed      Skip when the copy is fresh (source unchanged since last sync)
  --auto           --if-needed --quiet, and never fail (used by `just search` etc.)
  --dry-run        Show what would happen, change nothing
  -h, --help       Show this help

The copy runs even when the source profile is logged out; it only warns.

Environment: FME_CHROME_SRC, FME_PROFILE_NAME, FME_CHROME_DIR
EOF
    exit "${1:-0}"
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --profile) [ "$#" -ge 2 ] || { echo "missing value for --profile" >&2; exit 1; }; CLI_PROFILE="$2"; shift 2 ;;
        --source) [ "$#" -ge 2 ] || { echo "missing value for --source" >&2; exit 1; }; CLI_SOURCE="$2"; shift 2 ;;
        --dest) [ "$#" -ge 2 ] || { echo "missing value for --dest" >&2; exit 1; }; CLI_DEST="$2"; shift 2 ;;
        --if-needed) IF_NEEDED=1; shift ;;
        --auto) AUTO=1; IF_NEEDED=1; QUIET=1; shift ;;
        --detect) DETECT=1; shift ;;
        --force|-f) shift ;;  # kept for compatibility; copying no longer needs it
        --dry-run) DRY_RUN=1; shift ;;
        -h|--help) usage 0 ;;
        *) echo "unknown option: $1" >&2; usage 1 ;;
    esac
done

info() { [ "$QUIET" -eq 1 ] && return 0; printf '    %s\n' "$*"; }
ok() { [ "$QUIET" -eq 1 ] && return 0; printf '    \033[32m✓\033[0m %s\n' "$*"; }
warn() { [ "$QUIET" -eq 1 ] && return 0; printf '    \033[33m!\033[0m %s\n' "$*" >&2; }
die() {
    # In --auto mode a failed refresh must not break a scrape that still has a
    # usable copy; warn and let the scraper decide.
    if [ "$AUTO" -eq 1 ]; then
        printf '    \033[33m!\033[0m profile sync skipped: %s\n' "$*" >&2
        exit 0
    fi
    printf '\n\033[31merror:\033[0m %s\n' "$*" >&2
    exit 1
}

env_value() {
    [ -f "$ROOT_DIR/.env" ] || return 0
    sed -n "s/^$1=//p" "$ROOT_DIR/.env" 2>/dev/null | tail -1 | tr -d '\r'
}

# --- resolve configuration ---------------------------------------------------

case "$(uname -s)" in
    Darwin) default_chrome_src="$HOME/Library/Application Support/Google/Chrome" ;;
    Linux) default_chrome_src="${XDG_CONFIG_HOME:-$HOME/.config}/google-chrome" ;;
    *) default_chrome_src="" ;;
esac

env_dest="$(env_value FME_USER_DATA_DIR)"
env_dest_profile="$(env_value FME_PROFILE_DIRECTORY)"
env_profile_name="$(env_value FME_PROFILE_NAME)"
env_user="$(env_value FME_USERNAME)"
env_pass="$(env_value FME_PASSWORD)"

SRC="${CLI_SOURCE:-${FME_CHROME_SRC:-$default_chrome_src}}"
DEST="${CLI_DEST:-${FME_CHROME_DIR:-${env_dest:-$HOME/.local/share/fme-chrome}}}"
DEST_PROFILE="${env_dest_profile:-Default}"
PREFER_PROFILE="${CLI_PROFILE:-${FME_PROFILE_NAME:-$env_profile_name}}"
MARKER="$DEST/.fme-setup"

if [ -z "$DEST" ] || [ -z "$DEST_PROFILE" ]; then
    die "could not resolve the destination profile (set FME_USER_DATA_DIR / FME_PROFILE_DIRECTORY)"
fi

# Nothing to do when the scraper logs in with credentials and has no profile.
if [ -z "$env_dest" ] && [ -n "$env_user" ] && [ -n "$env_pass" ] && [ -z "$CLI_DEST" ]; then
    info ".env uses FME_USERNAME/FME_PASSWORD; nothing to sync"
    exit 0
fi

[ -n "$SRC" ] || die "no Chrome user-data dir could be resolved (set FME_CHROME_SRC)"
[ -d "$SRC" ] || die "Chrome user-data dir not found: $SRC"

DEST_PROFILE_DIR="$DEST/$DEST_PROFILE"

# --- cookie helpers ----------------------------------------------------------
# Read a private copy of the cookie DB (plus journal/WAL) so a running Chrome is
# never touched and writes still sitting in the WAL are visible.

cookie_db() {
    local prof="$1"
    if [ -f "$prof/Network/Cookies" ]; then
        printf '%s\n' "$prof/Network/Cookies"
    elif [ -f "$prof/Cookies" ]; then
        printf '%s\n' "$prof/Cookies"
    fi
}

fb_cookie_count() {
    local prof="$1" db tmpdir n
    db="$(cookie_db "$prof")"
    [ -n "$db" ] || { printf '0\n'; return 0; }

    tmpdir="$(mktemp -d)"
    cp -f "$db" "$tmpdir/Cookies" 2>/dev/null || true
    cp -f "$db-wal" "$tmpdir/Cookies-wal" 2>/dev/null || true
    cp -f "$db-shm" "$tmpdir/Cookies-shm" 2>/dev/null || true

    n=0
    if [ -f "$tmpdir/Cookies" ]; then
        if command -v sqlite3 >/dev/null 2>&1; then
            n="$(sqlite3 "$tmpdir/Cookies" \
                "select count(*) from cookies where name in ('c_user','xs') and host_key like '%facebook%';" \
                2>/dev/null || printf '0')"
        else
            grep -aqE 'c_user' "$tmpdir/Cookies" && n=1 || n=0
        fi
    fi
    rm -rf "$tmpdir"
    printf '%s\n' "${n:-0}"
}

has_fb_session() {
    local n
    n="$(fb_cookie_count "$1")"
    [ "${n:-0}" -gt 0 ] 2>/dev/null
}

file_mtime() {
    local f="$1"
    [ -f "$f" ] || { printf '0\n'; return 0; }
    if stat -c %Y "$f" >/dev/null 2>&1; then
        stat -c %Y "$f"
    else
        stat -f %m "$f" 2>/dev/null || printf '0\n'
    fi
}

# --- source profile selection ------------------------------------------------

real_profiles() {
    local entry name
    shopt -s nullglob
    for entry in "$SRC"/*/; do
        # Only real Chrome profiles have a Preferences file; the user-data dir
        # also holds component/cache directories we must skip.
        [ -f "$entry/Preferences" ] || continue
        name="$(basename "$entry")"
        case "$name" in "System Profile"|"Guest Profile") continue ;; esac
        printf '%s\n' "$name"
    done
    shopt -u nullglob
}

SOURCE_PROFILE="${PREFER_PROFILE:-$DEST_PROFILE}"

# --detect opts into the old behaviour: prefer Default when it has a login,
# otherwise the logged-in profile whose cookies were written most recently.
if [ "$DETECT" -eq 1 ]; then
    best=""
    best_mtime=-1
    default_found=0
    count=0
    candidates=""
    while IFS= read -r name; do
        [ -n "$name" ] || continue
        has_fb_session "$SRC/$name" || continue
        count=$((count + 1))
        candidates="$candidates        - $name"$'\n'
        if [ "$name" = "Default" ]; then default_found=1; fi
        mtime="$(file_mtime "$(cookie_db "$SRC/$name")")"
        if [ "$((${mtime:-0}))" -gt "$best_mtime" ] 2>/dev/null; then
            best="$name"
            best_mtime="${mtime:-0}"
        fi
    done <<< "$(real_profiles)"

    if [ "$default_found" -eq 1 ]; then
        SOURCE_PROFILE="Default"
    elif [ -n "$best" ]; then
        SOURCE_PROFILE="$best"
        if [ "$count" -gt 1 ]; then
            warn "several profiles have a Facebook login; using '$best' (pin one with --profile)"
            printf '%s' "$candidates" >&2
        fi
    else
        warn "no Chrome profile has a Facebook login; using '$SOURCE_PROFILE' anyway"
    fi
fi

[ -d "$SRC/$SOURCE_PROFILE" ] \
    || die "profile '$SOURCE_PROFILE' not found in $SRC (pick one with --profile)"

SOURCE_PROFILE_DIR="$SRC/$SOURCE_PROFILE"

# --- destination checks ------------------------------------------------------

if [ "$(cd -- "$SRC" && pwd)" = "$(cd -- "$DEST" 2>/dev/null && pwd || printf '%s' "$DEST")" ]; then
    die "source and destination are the same directory ($DEST)"
fi

source_db="$(cookie_db "$SOURCE_PROFILE_DIR")"
source_mtime="$(file_mtime "$source_db")"

# --- freshness check ---------------------------------------------------------

marker_profile=""
marker_source=""
marker_epoch=""
if [ -f "$MARKER" ]; then
    marker_profile="$(sed -n 's/^profile=//p' "$MARKER" | tail -1)"
    marker_source="$(sed -n 's/^source_profile=//p' "$MARKER" | tail -1)"
    marker_epoch="$(sed -n 's/^refreshed_epoch=//p' "$MARKER" | tail -1)"
fi

if [ "$IF_NEEDED" -eq 1 ]; then
    if [ "$marker_profile" = "$DEST_PROFILE" ] \
        && [ "$marker_source" = "$SOURCE_PROFILE" ] \
        && [ -n "$marker_epoch" ] \
        && [ "${source_mtime:-0}" -le "$marker_epoch" ] 2>/dev/null; then
        info "profile '$DEST_PROFILE' is up to date with '$SOURCE_PROFILE'; nothing to do"
        exit 0
    fi
fi

# --- copy --------------------------------------------------------------------

if [ -e "$DEST/SingletonLock" ]; then
    die "$DEST is in use by a running browser (SingletonLock present); close it and re-run"
fi
if pgrep -f -- "--user-data-dir=$DEST" >/dev/null 2>&1; then
    die "$DEST is in use by a running browser (user-data-dir in use); close it and re-run"
fi

info "source:      $SRC/$SOURCE_PROFILE ($( [ -n "$source_db" ] && printf '%s' "${source_db#$SOURCE_PROFILE_DIR/}" || printf 'no cookie db' ))"
info "destination: $DEST_PROFILE_DIR"

if [ "$DRY_RUN" -eq 1 ]; then
    ok "dry run: would copy '$SOURCE_PROFILE' into '$DEST/$DEST_PROFILE'"
    if ! has_fb_session "$SOURCE_PROFILE_DIR"; then
        warn "dry run: '$SOURCE_PROFILE' has no Facebook login; the scraper will not be authenticated"
    fi
    exit 0
fi

mkdir -p "$DEST_PROFILE_DIR"

# Local State carries the OS-keychain key material that decrypts the cookies.
if [ -f "$SRC/Local State" ]; then
    cp -f "$SRC/Local State" "$DEST/Local State"
fi

# Everything except the cookie stores and the heavy caches. rsync copies only
# what changed, so an unchanged profile refresh is cheap.
rsync -a \
    --exclude 'Cache/' --exclude 'Code Cache/' --exclude 'GPUCache/' \
    --exclude 'DawnCache/' --exclude 'GrShaderCache/' --exclude 'GraphiteDawnCache/' \
    --exclude 'ShaderCache/' --exclude 'Service Worker/' --exclude 'Crashpad/' \
    --exclude 'Crash Reports/' --exclude 'BrowserMetrics/' --exclude 'Safe Browsing/' \
    --exclude 'component_crx_cache/' --exclude 'extensions_crx_cache/' \
    --exclude 'Singleton*' --exclude 'lockfile' \
    --exclude 'Network/Cookies*' --exclude 'Cookies*' \
    "$SOURCE_PROFILE_DIR/" "$DEST_PROFILE_DIR/"

# The session itself: snapshot the cookie DB atomically so a copy taken while
# Chrome is running is still a consistent database. Stale journals next to the
# old DB must go, otherwise SQLite would replay them into the new file.
if [ -n "$source_db" ]; then
    rel="${source_db#"$SOURCE_PROFILE_DIR"/}"
    dest_db="$DEST_PROFILE_DIR/$rel"
    mkdir -p "$(dirname -- "$dest_db")"
    rm -f "$dest_db-journal" "$dest_db-wal" "$dest_db-shm"

    tmp_db="$(mktemp "$DEST_PROFILE_DIR/.cookies.XXXXXX")"
    if command -v sqlite3 >/dev/null 2>&1 \
        && sqlite3 "$source_db" ".backup '$tmp_db'" >/dev/null 2>&1 \
        && [ -s "$tmp_db" ]; then
        mv -f "$tmp_db" "$dest_db"
    else
        rm -f "$tmp_db"
        cp -f "$source_db" "$dest_db"
        if pgrep -f 'google-chrome|chromium' >/dev/null 2>&1; then
            warn "copied the cookie DB without a sqlite snapshot; quit Chrome for a guaranteed-consistent copy"
        fi
    fi
else
    warn "'$SOURCE_PROFILE' has no cookie store; copied files only"
fi

# --- marker + .env sanity ----------------------------------------------------

printf 'profile=%s\nsource_profile=%s\nsource=%s\nrefreshed_at=%s\nrefreshed_epoch=%s\n' \
    "$DEST_PROFILE" "$SOURCE_PROFILE" "$SRC" \
    "$(date -u +%FT%TZ)" "$(date -u +%s)" > "$MARKER"

env_dest_now="$(env_value FME_USER_DATA_DIR)"
if [ -n "$env_dest_now" ] && [ "$env_dest_now" != "$DEST" ]; then
    warn ".env points at '$env_dest_now' but the profile was copied to '$DEST'"
    warn "re-run 'just setup --force' if the destination changed"
fi

if [ "$(fb_cookie_count "$DEST_PROFILE_DIR")" -gt 0 ] 2>/dev/null; then
    ok "copied '$SOURCE_PROFILE' -> '$DEST/$DEST_PROFILE' (Facebook session present)"
else
    warn "copied '$SOURCE_PROFILE' -> '$DEST/$DEST_PROFILE', but it has no Facebook login"
    warn "log into facebook.com in that Chrome profile, then run: just sync-profile"
fi
