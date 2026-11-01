#!/usr/bin/env bash
#
# Shared Chrome-profile helpers for the fme scripts.
#
# Sourced by setup.sh, refresh-profile.sh and doctor.sh so the cookie check and the
# profile copy exist in exactly one place. Nothing here writes outside the paths the
# caller passes in.
#
# Globals set by fb_session_status():
#   FB_VALID    1 when the profile holds a usable Facebook session, else 0
#   FB_EXPIRY   epoch seconds when the session cookies expire (0 = unknown,
#               99999999999 = browser-session cookie)

# Chrome user-data dir for the current platform.
fme_default_chrome_src() {
    case "$(uname -s)" in
        Darwin) printf '%s\n' "$HOME/Library/Application Support/Google/Chrome" ;;
        Linux)  printf '%s\n' "${XDG_CONFIG_HOME:-$HOME/.config}/google-chrome" ;;
        *)      printf '%s\n' "${FME_CHROME_SRC:-}" ;;
    esac
}

# Read KEY from a dotenv-style file. Uses sed (not `source`) so values containing
# shell metacharacters (e.g. a password with '!') are never expanded.
fme_env_get() {
    local file="$1" key="$2" value=""
    [ -f "$file" ] || return 0
    value="$(sed -n "s/^${key}=//p" "$file" | tail -1)"
    case "$value" in
        \"*\") value="${value#\"}"; value="${value%\"}" ;;
        \'*\') value="${value#\'}"; value="${value%\'}" ;;
    esac
    printf '%s\n' "$value"
}

# Set KEY=VALUE in a dotenv-style file, replacing the last occurrence or appending.
# The caller is responsible for taking a backup first.
fme_env_set() {
    local file="$1" key="$2" value="$3" tmp
    tmp="$(mktemp)"
    awk -v key="$key" -v value="$value" '
        BEGIN { done = 0 }
        index($0, key "=") == 1 { print key "=" value; done = 1; next }
        { print }
        END { if (!done) print key "=" value }
    ' "$file" >"$tmp"
    mv "$tmp" "$file"
}

# Copy the cookie DB (plus WAL/SHM) into a temp dir so a running Chrome is never
# touched and recent writes still sitting in the WAL are visible. Prints the dir.
_fb_cookie_tmpdir() {
    local profdir="$1" tmpdir name
    tmpdir="$(mktemp -d)"
    for name in Cookies Cookies-wal Cookies-shm; do
        if [ -f "$profdir/Network/$name" ]; then
            cp -f "$profdir/Network/$name" "$tmpdir/$name" 2>/dev/null || true
        elif [ -f "$profdir/$name" ]; then
            cp -f "$profdir/$name" "$tmpdir/$name" 2>/dev/null || true
        fi
    done
    [ -f "$tmpdir/Cookies" ] || { rm -rf "$tmpdir"; return 1; }
    printf '%s\n' "$tmpdir"
}

# Is there a usable facebook.com session in PROFILE_DIR?
# Requires both c_user and xs, unexpired; expires_utc=0 means a browser-session
# cookie, which is still usable while that browser is running.
fb_session_status() {
    local profdir="$1" tmpdir db n=0 expiry=0
    FB_VALID=0
    FB_EXPIRY=0
    [ -d "$profdir" ] || return 1
    tmpdir="$(_fb_cookie_tmpdir "$profdir")" || return 1
    db="$tmpdir/Cookies"

    if command -v sqlite3 >/dev/null 2>&1; then
        n="$(sqlite3 "$db" \
            "select count(distinct name) from cookies
              where name in ('c_user','xs') and host_key like '%facebook%'
                and (expires_utc = 0
                     or expires_utc/1000000 - 11644473600 > CAST(strftime('%s','now') AS INTEGER));" \
            2>/dev/null || echo 0)"
        expiry="$(sqlite3 "$db" \
            "select coalesce(min(case when expires_utc = 0 then 99999999999
                                     else expires_utc/1000000 - 11644473600 end), 0)
               from cookies
              where name in ('c_user','xs') and host_key like '%facebook%'
                and (expires_utc = 0
                     or expires_utc/1000000 - 11644473600 > CAST(strftime('%s','now') AS INTEGER));" \
            2>/dev/null || echo 0)"
    else
        # Without sqlite3 we can only check for presence, not expiry.
        grep -aq 'c_user' "$db" && n=2
    fi

    rm -rf "$tmpdir"
    [ "${n:-0}" -gt 1 ] 2>/dev/null || return 1
    FB_VALID=1
    FB_EXPIRY="${expiry:-0}"
    return 0
}

# Human-readable expiry for an epoch produced by fb_session_status.
fme_epoch_to_date() {
    local epoch="${1:-0}"
    if [ "$epoch" = "99999999999" ]; then
        printf 'browser session (not persisted)\n'
        return 0
    fi
    if [ "${epoch:-0}" -le 0 ] 2>/dev/null; then
        printf 'unknown\n'
        return 0
    fi
    date -r "$epoch" -u +%Y-%m-%d 2>/dev/null \
        || date -u -d "@$epoch" +%Y-%m-%d 2>/dev/null \
        || printf 'unknown\n'
}

# Print every Chrome profile under SRC that currently holds a usable FB session.
list_fb_profiles() {
    local src="$1" entry name found=""
    [ -d "$src" ] || return 1
    shopt -s nullglob
    for entry in "$src"/*/; do
        # Only real Chrome profiles have a Preferences file; the user-data dir also
        # holds component/cache directories we must skip.
        [ -f "$entry/Preferences" ] || continue
        name="$(basename "$entry")"
        case "$name" in "System Profile" | "Guest Profile") continue ;; esac
        if fb_session_status "$entry"; then
            found=1
            printf '        %-16s session expires: %s\n' \
                "$name" "$(fme_epoch_to_date "$FB_EXPIRY")"
        fi
    done
    shopt -u nullglob
    [ -n "$found" ]
}

# Print the profile under SRC with the usable FB session that expires last.
best_fb_profile() {
    local src="$1" entry name best="" best_exp=0 score=0
    [ -d "$src" ] || return 1
    shopt -s nullglob
    for entry in "$src"/*/; do
        [ -f "$entry/Preferences" ] || continue
        name="$(basename "$entry")"
        case "$name" in "System Profile" | "Guest Profile") continue ;; esac
        # Prefer a persistent cookie over a browser-session one.
        if fb_session_status "$entry"; then
            score="$FB_EXPIRY"
            [ "$score" = "99999999999" ] && score=1
            if [ "$score" -gt "$best_exp" ] 2>/dev/null; then
                best="$name"
                best_exp="$score"
            fi
        fi
    done
    shopt -u nullglob
    [ -n "$best" ] || return 1
    printf '%s\n' "$best"
}

# Is a Chrome instance currently using the user-data dir DIR?
profile_in_use() {
    local dir="$1" lock target pid
    lock="$dir/SingletonLock"
    if [ -L "$lock" ]; then
        # Chrome points SingletonLock at "<hostname>-<pid>".
        target="$(readlink "$lock" 2>/dev/null || true)"
        pid="${target##*-}"
        case "$pid" in
            '' | *[!0-9]*) ;;
            *) kill -0 "$pid" 2>/dev/null && return 0 ;;
        esac
    fi
    if command -v pgrep >/dev/null 2>&1; then
        pgrep -f -- "--user-data-dir=$dir" >/dev/null 2>&1 && return 0
    fi
    return 1
}

# Copy SRC_PROFILE into DST_ROOT/NAME via a staging dir, verify the copy holds a
# usable Facebook session, then swap it into place. A failed or torn copy leaves
# the previous profile untouched.
copy_profile() {
    local src_profile="$1" dst_root="$2" name="$3" verify="${4:-1}"
    local staging old
    staging="$dst_root/.fme-staging-$$"
    old="$dst_root/$name"

    mkdir -p "$dst_root" || return 1
    rm -rf "$staging"
    mkdir -p "$staging" || return 1

    if ! rsync -a \
        --exclude 'Cache/' --exclude 'Code Cache/' --exclude 'GPUCache/' \
        --exclude 'DawnCache/' --exclude 'GrShaderCache/' --exclude 'GraphiteDawnCache/' \
        --exclude 'ShaderCache/' --exclude 'Service Worker/' --exclude 'Crashpad/' \
        --exclude 'Crash Reports/' --exclude 'BrowserMetrics/' --exclude 'Safe Browsing/' \
        --exclude 'component_crx_cache/' --exclude 'extensions_crx_cache/' \
        --exclude 'Singleton*' --exclude 'lockfile' \
        "$src_profile/" "$staging/"; then
        rm -rf "$staging"
        return 1
    fi

    if [ "$verify" -eq 1 ] && ! fb_session_status "$staging"; then
        rm -rf "$staging"
        return 1
    fi

    rm -rf "$old.fme-old"
    if [ -e "$old" ] && ! mv "$old" "$old.fme-old"; then
        rm -rf "$staging"
        return 1
    fi
    if ! mv "$staging" "$old"; then
        [ -d "$old.fme-old" ] && mv "$old.fme-old" "$old"
        rm -rf "$staging"
        return 1
    fi
    rm -rf "$old.fme-old"
    return 0
}

# Copy the sibling "Local State" file (holds the profile list Chrome resolves a
# --profile-directory against). Best effort: a missing file is not fatal.
copy_local_state() {
    local src_root="$1" dst_root="$2"
    [ -f "$src_root/Local State" ] || return 0
    mkdir -p "$dst_root" 2>/dev/null || return 0
    cp -f "$src_root/Local State" "$dst_root/Local State" 2>/dev/null || return 0
}
