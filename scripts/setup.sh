#!/usr/bin/env bash
#
# One-time bootstrap for a fresh clone: log-in profile, .env, dependencies, build and
# the Playwright/Chromium download. Safe to re-run: every step is idempotent and only
# --force / --refresh-profile overwrite existing state.
#
# Usage: just setup [--force] [--refresh-profile] [--skip-deps] [--skip-build] [--skip-drivers]
#
# Configuration (environment variables, all optional):
#   FME_CHROME_SRC     Chrome user-data dir to copy the profile from
#   FME_PROFILE_NAME   Chrome profile to copy (default: Default)
#   FME_CHROME_DIR     Destination user-data dir for the scraper
#   FME_USERNAME       Facebook email; when set (with FME_PASSWORD) the profile copy
#   FME_PASSWORD       is skipped and the tool logs in with credentials instead
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"

FORCE=0
REFRESH_PROFILE=0
SKIP_DEPS=0
SKIP_BUILD=0
SKIP_DRIVERS=0

usage() {
    cat <<'EOF'
One-time bootstrap for a fresh clone: log-in profile, .env, dependencies, build and
the Playwright/Chromium download. Safe to re-run: every step is idempotent and only
--force / --refresh-profile overwrite existing state.

Usage: just setup [--force] [--refresh-profile] [--skip-deps] [--skip-build] [--skip-drivers]

Configuration (environment variables, all optional):
  FME_CHROME_SRC     Chrome user-data dir to copy the profile from
  FME_PROFILE_NAME   Chrome profile to copy (default: Default)
  FME_CHROME_DIR     Destination user-data dir for the scraper
  FME_USERNAME       Facebook email; when set (with FME_PASSWORD) the profile copy
  FME_PASSWORD       is skipped and the tool logs in with credentials instead
EOF
    exit "${1:-0}"
}

for arg in "$@"; do
    case "$arg" in
        --force|-f) FORCE=1 ;;
        --refresh-profile) REFRESH_PROFILE=1 ;;
        --skip-deps) SKIP_DEPS=1 ;;
        --skip-build) SKIP_BUILD=1 ;;
        --skip-drivers) SKIP_DRIVERS=1 ;;
        -h|--help) usage 0 ;;
        *) echo "unknown option: $arg" >&2; usage 1 ;;
    esac
done

step() { printf '\n\033[1;34m==>\033[0m \033[1m%s\033[0m\n' "$*"; }
info() { printf '    %s\n' "$*"; }
ok() { printf '    \033[32m✓\033[0m %s\n' "$*"; }
warn() { printf '    \033[33m!\033[0m %s\n' "$*"; }
die() { printf '\n\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# --- platform defaults -------------------------------------------------------

case "$(uname -s)" in
    Darwin)
        default_chrome_src="$HOME/Library/Application Support/Google/Chrome"
        ;;
    Linux)
        default_chrome_src="${XDG_CONFIG_HOME:-$HOME/.config}/google-chrome"
        ;;
    *)
        default_chrome_src="${FME_CHROME_SRC:-}"
        ;;
esac

FME_CHROME_SRC="${FME_CHROME_SRC:-$default_chrome_src}"
FME_PROFILE_NAME="${FME_PROFILE_NAME:-Default}"
FME_CHROME_DIR="${FME_CHROME_DIR:-$HOME/.local/share/fme-chrome}"
MARKER="$FME_CHROME_DIR/.fme-setup"

cd "$ROOT_DIR"

printf '\033[1mfacebook marketplace extractor — setup\033[0m\n'
info "repo:           $ROOT_DIR"
info "chrome source:  ${FME_CHROME_SRC:-<unset>}"
info "profile:        $FME_PROFILE_NAME"
info "chrome dir:     $FME_CHROME_DIR"

# --- 1. prerequisites --------------------------------------------------------

step "Checking prerequisites"

command -v go >/dev/null 2>&1 || die "go is not installed (need >= 1.25): https://go.dev/dl/"
go_version="$(go env GOVERSION | sed 's/^go//')"
required_go="$(sed -n 's/^go \([0-9.]*\)$/\1/p' "$ROOT_DIR/go.work" | head -1)"
info "go $go_version (workspace requires >= ${required_go:-1.25})"

command -v just >/dev/null 2>&1 || die "just is not installed: https://github.com/casey/just#installation"
ok "just $(just --version | awk '{print $2}')"

chrome_bin=""
for candidate in google-chrome google-chrome-stable chromium chromium-browser; do
    if command -v "$candidate" >/dev/null 2>&1; then chrome_bin="$candidate"; break; fi
done
if [ -z "$chrome_bin" ] && [ -x "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ]; then
    chrome_bin="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
fi
[ -n "$chrome_bin" ] || die "Google Chrome was not found on PATH; install it before running setup"
ok "chrome: $chrome_bin"

command -v rsync >/dev/null 2>&1 || die "rsync is required to copy the Chrome profile"
command -v sqlite3 >/dev/null 2>&1 && ok "sqlite3 present" || warn "sqlite3 not found; falling back to a coarse cookie check"

# --- cookie inspection -------------------------------------------------------
# Copy the DB (plus WAL/SHM) so a running Chrome is never touched and recent
# writes sitting in the WAL are still visible.
fb_session_in() {
    local profdir="$1" tmpdir n=0 name
    [ -d "$profdir" ] || return 1

    tmpdir="$(mktemp -d)"
    for name in Cookies Cookies-wal Cookies-shm; do
        [ -f "$profdir/Network/$name" ] && cp -f "$profdir/Network/$name" "$tmpdir/$name" 2>/dev/null || true
    done
    if [ ! -f "$tmpdir/Cookies" ]; then
        for name in Cookies Cookies-wal Cookies-shm; do
            [ -f "$profdir/$name" ] && cp -f "$profdir/$name" "$tmpdir/$name" 2>/dev/null || true
        done
    fi

    if [ -f "$tmpdir/Cookies" ]; then
        if command -v sqlite3 >/dev/null 2>&1; then
            n="$(sqlite3 "$tmpdir/Cookies" \
                "select count(*) from cookies where name in ('c_user','xs') and host_key like '%facebook%';" \
                2>/dev/null || echo 0)"
        else
            grep -aqE 'c_user' "$tmpdir/Cookies" && n=1 || n=0
        fi
    fi
    rm -rf "$tmpdir"
    [ "${n:-0}" -gt 0 ] 2>/dev/null
}

# Print every profile that currently holds a Facebook session.
list_fb_profiles() {
    local src="$1" entry name
    shopt -s nullglob
    for entry in "$src"/*/; do
        # Only real Chrome profiles have a Preferences file; the user-data dir also
        # holds component/cache directories we must skip.
        [ -f "$entry/Preferences" ] || continue
        name="$(basename "$entry")"
        case "$name" in System\ Profile|Guest\ Profile) continue ;; esac
        if fb_session_in "$entry"; then printf '        - %s\n' "$name"; fi
    done
    shopt -u nullglob
}

# --- 2. profile / credentials ------------------------------------------------

step "Preparing the Facebook session"

if [ -n "${FME_USERNAME:-}" ] && [ -n "${FME_PASSWORD:-}" ]; then
    info "FME_USERNAME/FME_PASSWORD are set; skipping the Chrome profile copy"
    USE_CREDENTIALS=1
else
    USE_CREDENTIALS=0
    [ -n "$FME_CHROME_SRC" ] && [ -d "$FME_CHROME_SRC" ] \
        || die "Chrome user-data dir not found: '${FME_CHROME_SRC:-<unset>}' (set FME_CHROME_SRC)"

    profile_src="$FME_CHROME_SRC/$FME_PROFILE_NAME"
    [ -d "$profile_src" ] || die "profile '$FME_PROFILE_NAME' not found in $FME_CHROME_SRC (set FME_PROFILE_NAME)"

    if ! fb_session_in "$profile_src"; then
        printf '\n\033[31merror:\033[0m profile '\''%s'\'' has no Facebook login.\n\n' "$FME_PROFILE_NAME" >&2
        echo "Log into Facebook once, then re-run 'just setup':" >&2
        echo "  1. open Chrome and sign in at https://www.facebook.com" >&2
        echo "  2. quit Chrome completely (otherwise the profile is locked)" >&2
        echo "  3. just setup" >&2
        echo >&2
        echo "Profiles with a Facebook session found:" >&2
        found="$(list_fb_profiles "$FME_CHROME_SRC")"
        if [ -n "$found" ]; then
            printf '%s\n' "$found" >&2
            echo >&2
            echo "Use one of them with: FME_PROFILE_NAME='<name>' just setup" >&2
        else
            echo "        (none)" >&2
        fi
        exit 1
    fi
    ok "profile '$FME_PROFILE_NAME' has a Facebook session"

    marker_profile=""
    if [ -f "$MARKER" ]; then
        marker_profile="$(sed -n 's/^profile=//p' "$MARKER" | tail -1)"
    fi

    if [ "$marker_profile" = "$FME_PROFILE_NAME" ] && [ "$REFRESH_PROFILE" -eq 0 ]; then
        ok "profile '$FME_PROFILE_NAME' already copied to $FME_CHROME_DIR (use --refresh-profile to refresh)"
    else
        if [ -f "$MARKER" ] && [ "$marker_profile" != "$FME_PROFILE_NAME" ]; then
            warn "copied profile is '$marker_profile', switching to '$FME_PROFILE_NAME'"
        fi
        if [ -e "$FME_CHROME_DIR/SingletonLock" ]; then
            die "$FME_CHROME_DIR is in use by a running browser (SingletonLock present); close it and re-run"
        fi
        if pgrep -f '/opt/google/chrome/chrome|Google Chrome.app|google-chrome' >/dev/null 2>&1; then
            warn "Chrome is running; the copy is usually fine, but quit it for a guaranteed-consistent session"
        fi

        mkdir -p "$FME_CHROME_DIR"
        info "copying 'Local State' + profile '$FME_PROFILE_NAME' (caches excluded)..."
        cp -f "$FME_CHROME_SRC/Local State" "$FME_CHROME_DIR/Local State" 2>/dev/null || true
        rsync -a \
            --exclude 'Cache/' --exclude 'Code Cache/' --exclude 'GPUCache/' \
            --exclude 'DawnCache/' --exclude 'GrShaderCache/' --exclude 'GraphiteDawnCache/' \
            --exclude 'ShaderCache/' --exclude 'Service Worker/' --exclude 'Crashpad/' \
            --exclude 'Crash Reports/' --exclude 'BrowserMetrics/' --exclude 'Safe Browsing/' \
            --exclude 'component_crx_cache/' --exclude 'extensions_crx_cache/' \
            --exclude 'Singleton*' --exclude 'lockfile' \
            "$profile_src/" "$FME_CHROME_DIR/$FME_PROFILE_NAME/"

        printf 'profile=%s\nsource=%s\nrefreshed_at=%s\n' \
            "$FME_PROFILE_NAME" "$FME_CHROME_SRC" "$(date -u +%FT%TZ)" > "$MARKER"
        ok "profile copied to $FME_CHROME_DIR"
    fi
fi

# --- 3. .env -----------------------------------------------------------------

step "Writing .env"

env_file="$ROOT_DIR/.env"
write_env=1

if [ -f "$env_file" ] && [ "$FORCE" -eq 0 ]; then
    if [ "$USE_CREDENTIALS" -eq 1 ]; then
        ok ".env already exists (use --force to regenerate)"
    elif grep -q "^FME_USER_DATA_DIR=$FME_CHROME_DIR$" "$env_file" \
        && grep -q "^FME_PROFILE_DIRECTORY=$FME_PROFILE_NAME$" "$env_file"; then
        ok ".env already points at $FME_CHROME_DIR (profile '$FME_PROFILE_NAME')"
    else
        warn ".env exists but does not match $FME_CHROME_DIR / profile '$FME_PROFILE_NAME'"
        warn "re-run with --force to regenerate it (a backup is kept as .env.bak)"
    fi
    write_env=0
fi

if [ "$write_env" -eq 1 ]; then
    if [ -f "$env_file" ]; then
        cp -f "$env_file" "$env_file.bak"
        info "existing .env backed up to .env.bak"
    fi
    if [ "$USE_CREDENTIALS" -eq 1 ]; then
        cat > "$env_file" <<EOF
# Generated by 'just setup'. Prefer a logged-in Chrome profile (FME_USER_DATA_DIR):
# credentials trigger a manual captcha on every new session.
FME_USERNAME=$FME_USERNAME
FME_PASSWORD=$FME_PASSWORD
EOF
    else
        cat > "$env_file" <<EOF
# Generated by 'just setup'.
# The scraper launches Chrome with this copy of your logged-in profile.
# Refresh it with: just sync-profile
FME_USER_DATA_DIR=$FME_CHROME_DIR
FME_BROWSER_CHANNEL=chrome
FME_PROFILE_DIRECTORY=$FME_PROFILE_NAME
EOF
    fi
    ok "wrote $env_file"
fi

# --- 4. dependencies + build -------------------------------------------------

if [ "$SKIP_DEPS" -eq 0 ]; then
    step "Downloading Go dependencies"
    just tidy
    ok "modules tidied"
fi

if [ "$SKIP_BUILD" -eq 0 ]; then
    step "Building the scraper"
    just build
    ok "built $ROOT_DIR/fme"
fi

# --- 5. playwright driver + chromium ----------------------------------------

if [ "$SKIP_DRIVERS" -eq 0 ]; then
    step "Installing the Playwright driver and Chromium"
    go run ./go/cmd/fme -action=install_drivers
    ok "browser runtime ready"
else
    warn "skipped the Playwright install; the first scrape will download it instead"
fi

# --- done --------------------------------------------------------------------

step "Setup complete"
cat <<EOF
    Verify with:   just doctor
    Collect data:  just search "macbook m1"
    Descriptions:  just pull-description
    Web UI:        just serve     (http://127.0.0.1:8090/_/)
EOF
