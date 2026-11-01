#!/usr/bin/env bash
#
# Read-only diagnosis of a partial or broken setup. Never modifies anything.
# Every failed check prints the command that fixes it.
#
# Usage: just doctor
set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

FAILURES=0
WARNINGS=0

pass() { printf '  \033[32m✓\033[0m %s\n' "$*"; }
fail() { printf '  \033[31m✗\033[0m %s\n' "$*"; FAILURES=$((FAILURES + 1)); }
warn() { printf '  \033[33m!\033[0m %s\n' "$*"; WARNINGS=$((WARNINGS + 1)); }
fix() { printf '      \033[2mfix: %s\033[0m\n' "$*"; }
section() { printf '\n\033[1m%s\033[0m\n' "$*"; }

printf '\033[1mfacebook marketplace extractor — doctor\033[0m\n'

# --- toolchain ---------------------------------------------------------------

section "Toolchain"

if command -v go >/dev/null 2>&1; then
    pass "go $(go env GOVERSION | sed 's/^go//')"
else
    fail "go not found"
    fix "install Go >= 1.25 from https://go.dev/dl/"
fi

if command -v just >/dev/null 2>&1; then
    pass "just $(just --version | awk '{print $2}')"
else
    fail "just not found"
    fix "install just: https://github.com/casey/just#installation"
fi

if command -v google-chrome >/dev/null 2>&1 || command -v chromium >/dev/null 2>&1 \
    || [ -x "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ]; then
    pass "Google Chrome is installed"
else
    fail "Google Chrome not found on PATH"
    fix "install Google Chrome (the scraper drives the real Chrome via channel=chrome)"
fi

# --- .env --------------------------------------------------------------------

section "Configuration (.env)"

env_file="$ROOT_DIR/.env"
user_data_dir=""

if [ -f "$env_file" ]; then
    pass ".env exists"
    # shellcheck disable=SC1090
    user_data_dir="$(sed -n 's/^FME_USER_DATA_DIR=//p' "$env_file" | tail -1)"
    profile_directory="$(sed -n 's/^FME_PROFILE_DIRECTORY=//p' "$env_file" | tail -1)"
    channel="$(sed -n 's/^FME_BROWSER_CHANNEL=//p' "$env_file" | tail -1)"

    if [ -n "$user_data_dir" ]; then
        pass "FME_USER_DATA_DIR=$user_data_dir"
        [ -n "$profile_directory" ] && pass "FME_PROFILE_DIRECTORY=$profile_directory" \
            || warn "FME_PROFILE_DIRECTORY unset; defaults to the profile in Local State"
        [ -n "$channel" ] && pass "FME_BROWSER_CHANNEL=$channel" \
            || warn "FME_BROWSER_CHANNEL unset; system Chromium will be used instead of Chrome"
    elif grep -q '^FME_USERNAME=' "$env_file"; then
        warn "using credential login (FME_USERNAME); a captcha must be solved on every new session"
    else
        fail ".env has neither FME_USER_DATA_DIR nor FME_USERNAME"
        fix "just setup"
    fi
else
    fail ".env is missing (the scraper exits immediately without it)"
    fix "just setup"
fi

# --- chrome profile ----------------------------------------------------------

section "Chrome profile"

if command -v sqlite3 >/dev/null 2>&1; then
    has_fb_session() {
        local profdir="$1" tmpdir n=0 name
        [ -d "$profdir" ] || return 1
        tmpdir="$(mktemp -d)"
        for name in Cookies Cookies-wal Cookies-shm; do
            [ -f "$profdir/Network/$name" ] && cp -f "$profdir/Network/$name" "$tmpdir/$name" 2>/dev/null
            [ -f "$profdir/$name" ] && cp -f "$profdir/$name" "$tmpdir/$name" 2>/dev/null
        done
        if [ -f "$tmpdir/Cookies" ]; then
            n="$(sqlite3 "$tmpdir/Cookies" \
                "select count(*) from cookies where name='c_user' and host_key like '%facebook%';" 2>/dev/null || echo 0)"
        fi
        rm -rf "$tmpdir"
        [ "${n:-0}" -gt 0 ] 2>/dev/null
    }

    if [ -n "$user_data_dir" ] && [ -d "$user_data_dir" ]; then
        pass "profile dir exists: $user_data_dir"
        if [ -n "${profile_directory:-}" ] && has_fb_session "$user_data_dir/$profile_directory"; then
            pass "profile '$profile_directory' has a Facebook login"
        elif has_fb_session "$user_data_dir/Default"; then
            pass "profile 'Default' has a Facebook login"
        else
            fail "no Facebook login in $user_data_dir"
            fix "log into facebook.com in Chrome, then: just sync-profile"
        fi
    elif [ -n "$user_data_dir" ]; then
        fail "profile dir does not exist: $user_data_dir"
        fix "just setup"
    fi
else
    warn "sqlite3 not found; cannot verify the Facebook session"
fi

# --- playwright --------------------------------------------------------------

section "Browser runtime (Playwright)"

driver_cache="${PLAYWRIGHT_DRIVER_PATH:-$HOME/.cache/ms-playwright-go}"
browser_cache="${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}"

if [ -d "$driver_cache" ]; then
    pass "driver cache: $driver_cache"
else
    warn "Playwright driver cache missing ($driver_cache)"
    fix "just run -action=install_drivers"
fi

if [ -d "$browser_cache" ] && find "$browser_cache" -maxdepth 1 -name 'chromium-*' 2>/dev/null | grep -q .; then
    pass "Chromium installed in $browser_cache"
else
    warn "Chromium not found in $browser_cache"
    fix "just run -action=install_drivers"
fi

# --- build artifacts ---------------------------------------------------------

section "Build and data"

if [ -x "$ROOT_DIR/fme" ]; then
    pass "binary built: fme"
else
    warn "binary not built"
    fix "just build"
fi

detail_dir="$ROOT_DIR/data/__Details"
if [ -d "$detail_dir" ]; then
    pass "details: $(find "$detail_dir" -name 'detail_*.json' | wc -l | tr -d ' ') listing(s) in data/__Details"
else
    warn "no data/__Details yet"
    fix "just search \"macbook m1\""
fi

if [ -d "$ROOT_DIR/pb_data" ]; then
    pass "PocketBase data dir present (pb_data)"
else
    warn "pb_data missing; it is created on first use"
    fix "just save  (imports data/__Details; browse it with: just serve)"
fi

# --- summary -----------------------------------------------------------------

printf '\n'
if [ "$FAILURES" -eq 0 ] && [ "$WARNINGS" -eq 0 ]; then
    printf '\033[32meverything looks good.\033[0m\n'
elif [ "$FAILURES" -eq 0 ]; then
    printf '\033[33m%s warning(s), no blocking problems.\033[0m\n' "$WARNINGS"
else
    printf '\033[31m%s blocking problem(s), %s warning(s).\033[0m\n' "$FAILURES" "$WARNINGS"
fi

exit "$([ "$FAILURES" -eq 0 ] && echo 0 || echo 1)"
