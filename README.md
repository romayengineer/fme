# Facebook Marketplace Extractor

Pulls product listings from Facebook Marketplace by driving your real Chrome with
Playwright, intercepting Facebook's GraphQL responses, and materializing them as JSON
(`data/__Details/detail_<id>.json`) plus an optional PocketBase database.

The most valuable field, `redacted_description`, is **not** in the search feed: it comes
from a separate Product Detail Page (PDP) request. See
[docs/redacted_description.md](docs/redacted_description.md) for how that works.

Heat maps of the collected listings are rendered by the `analytics` module. Some
cities resolve to a single Facebook coordinate and create misleadingly dense cells;
see [docs/location_geocoding_outliers.md](docs/location_geocoding_outliers.md).

## Quick start

```bash
git clone <repo> && cd fme
just setup          # one time, right after cloning
just doctor         # verify the setup
just search "macbook m1"
just pull-description
just save           # import into PocketBase (creates pb_data)
just serve          # browse at http://127.0.0.1:8090/_/
```

`just setup` is idempotent — re-run it any time; it only does work that is missing.

## Requirements

- **Go >= 1.25** — https://go.dev/dl/
- **just** — https://github.com/casey/just#installation
- **Google Chrome**, signed into `facebook.com` at least once
- `rsync`; `sqlite3` is optional (used to verify the session)
- First run downloads the Playwright driver + Chromium (~200–300 MB) into
  `~/.cache/ms-playwright-go`

The Facebook session is the one piece of manual setup: open Chrome, log in once, then
run `just setup`. `setup` copies that profile into `~/.local/share/fme-chrome` so the
scraper never touches (or fights with) your everyday Chrome profile.

## What `just setup` does

1. Checks prerequisites (`go`, `just`, Chrome, `rsync`).
2. Finds a Chrome profile with a Facebook login and copies it to
   `~/.local/share/fme-chrome` (caches excluded).
3. Writes `.env` (`FME_USER_DATA_DIR`, `FME_BROWSER_CHANNEL`, `FME_PROFILE_DIRECTORY`).
4. `just tidy` + `just build`.
5. Installs the Playwright driver and Chromium.

Flags: `--force` (regenerate `.env`, keeping `.env.bak`), `--refresh-profile` (re-copy
the Chrome profile), `--skip-deps`, `--skip-build`, `--skip-drivers`.

Overridable settings (env vars): `FME_CHROME_SRC`, `FME_PROFILE_NAME`,
`FME_CHROME_DIR`. To target a different profile:

```bash
FME_PROFILE_NAME="Profile 9" just setup
```

If no profile has a Facebook session, `setup` lists the candidates it found and tells
you to log in first. Without a profile it also accepts `FME_USERNAME`/`FME_PASSWORD`
(credential login, but a captcha must be solved on each new session).

## Commands

| Command | Description |
|---|---|
| `just setup` | One-time (idempotent) bootstrap |
| `just doctor` | Diagnose a partial/broken setup (read-only) |
| `just search "macbook m1"` | Collect listings for a keyword into `data/__Details` |
| `just pull-description` | Fill missing `Description` fields via the PDP request |
| `just process-data` | Re-extract listings from captured responses (no browser) |
| `just description-stats` | Report how many `data/__Details` listings have no description |
| `just dedupe-pdp` | Remove duplicate PDP responses (keeps newest per id) |
| `just serve` | PocketBase admin/web UI on http://127.0.0.1:8090/_/ |
| `just save` | Import `data/__Details` into PocketBase (creates the DB if needed) |
| `just sync` | Push `data/` to a remote host (merge; see below) |
| `just build` / `just test` / `just lint` / `just vet` | Build and quality gates |
| `just ml-install` / `just ml-train` | Python price-model pipeline |

`just search` pauses on `waiting for input:` — press Enter to finish.

## Syncing data to a server

`just sync` rsyncs the scraped `data/` directory to a remote machine that has the
same repo/branch cloned. It **merges by default** — files that exist only on the
remote are kept — so it is safe to re-run.

```bash
just sync --dry-run        # show what would transfer
just sync                  # push local data/ to the remote
just sync --save           # push, then `just save` on the remote (import to PocketBase)
just sync --pull           # pull the remote data/ back down
just sync --delete         # mirror exactly (removes remote-only files — destructive)
```

Target defaults to `maxi@192.168.0.235` with the project at
`~/Projects/romayengineer/fme`, and can be set with the
`FME_SYNC_REMOTE` / `FME_SYNC_REMOTE_DIR` environment variables (or `.env`) or
overridden per-run with `--host` / `--dir`. The `.env` is never copied: each
machine keeps its own Chrome-profile path.

## Troubleshooting

Run `just doctor` first; it prints the exact fix for anything it finds.

- **"not logged in (url: …)"** — the copied session expired. Log into Facebook in
  Chrome, quit Chrome, then `just setup --refresh-profile`.
- **Profile dir is in use / launch fails** — close the browser using that profile; a
  stale `SingletonLock` in `~/.local/share/fme-chrome` blocks the launch.
- **`pulled=0` / error `1675004`** — Facebook is throttling the account. Wait, then
  re-run `just pull-description` (optionally `-delay-ms=2000`). Progress is saved.
- **Chromium download fails** — network/proxy issue; `just run -action=install_drivers`
  retries it, or set `PLAYWRIGHT_DRIVER_PATH` to a shared cache.
- **Missing Chrome shared libraries** — `just setup` only downloads Chromium; on bare
  Linux images you may also need the system deps (`playwright install-deps chromium`).

## Layout

```
go/cmd/fme            CLI entrypoint (flags, actions, orchestration)
go/browser/           Playwright driver wrapper, Facebook login, page helpers
go/capture/           Response interception and PDP request replay
go/marketplace/model  Product model and URL helpers
go/store/filestore    data/ JSON store + extraction pipeline (ProcessData)
go/store/db           PocketBase integration (serve, save)
go/core/              Logging, JSON, codec, string utilities
ml/                   Python price-prediction pipeline
scripts/              setup.sh, doctor.sh
```
