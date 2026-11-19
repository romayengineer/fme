# Running the scraper unattended

`fme -action=schedule` runs the scraper in a loop: it searches every term in
`search-terms.txt`, pulls the descriptions of newly discovered listings, tidies
the data directory, and sleeps until the next cycle. If Facebook temporarily
throttles the account (`1675004` / the "page isn't available" error page), the
loop backs off and resumes automatically instead of hammering the API.

The intended deployment is a **systemd user service**, so the process starts
with your login session (which is what gives Chrome access to the keyring that
decrypts the copied Facebook cookies).

## Quick start

```bash
just setup                 # once, if the Chrome profile is not ready
just build                 # builds ./fme for the service
$EDITOR search-terms.txt   # your Marketplace queries, one per line
just daemon -once          # try one cycle in the foreground
just service-install       # install + start the systemd user service
just status                # last cycle, next run, backoff, last error
just service-logs          # follow the journal
```

To stop it again:

```bash
systemctl --user stop fme-schedule.service
just service-install --uninstall   # stop, disable, and remove the unit
```

## Terms

`search-terms.txt` (repo root) holds one Marketplace query per line. Blank
lines and lines starting with `#` are ignored, and duplicate terms are dropped.

The scheduler reads the file once at startup. After editing it, restart the
service so the new terms are loaded:

```bash
systemctl --user restart fme-schedule.service
```

Keep the list short and specific. Each term costs one search (up to
`-scroll-seconds`, default 240, of scrolling while the feed keeps growing) and
adds listings whose descriptions are pulled in the same search pass. A catch-up
description pass still runs after all terms.

## What a cycle does

1. Refreshes the copied Chrome profile (`scripts/sync-profile.sh --auto`), so a
   re-login in normal Chrome is picked up without manual work.
2. Searches every term, pausing `-per-term-delay` between consecutive searches.
   Each search also pulls descriptions for the listings it discovers through a
   bounded FIFO (queue limit 50); scrolling pauses while the queue is full.
3. Runs a catch-up description pass for any listing still missing one, pausing
   `-schedule-delay-ms` between replayed PDP requests.
4. Dedupes PDP responses and archives processed search responses.
5. Writes `data/scheduler-state.json` and sleeps until the next cycle
   (`-interval`, default 6h).

Progress is saved as it happens, so a crash or a rate limit never loses the
descriptions already fetched.

## Rate limits and backoff

When a search page or a PDP request reports throttling, the cycle stops and the
state records a `blockedUntil` timestamp. The retry delay grows exponentially
with no cap below **2 hours**:

| Block | Wait before retry |
|---|---|
| 1st | 15 minutes |
| 2nd | 30 minutes |
| 3rd | 1 hour |
| 4th and later | 2 hours |

A successful request resets the backoff. The persisted `blockedUntil` means a
service restart during a block does not immediately hammer Facebook again.
Other errors (browser launch, expired session) are logged and retried after 15
minutes without stopping the process.

## State and status

`data/scheduler-state.json` is the durable status file (atomic writes, ignored
by git). `just status` renders it:

```
cycles:              12
last cycle:          2026-11-02T18:00:00-03:00
next run:            2026-11-03T00:00:00-03:00
blocked until:       never
last success:        2026-11-02T17:52:31-03:00
last throttled:      never
backoff:             0s
last term:           mac mini
descriptions pulled: 84 (pending 0)
last error:
```

Only one scheduler may run at a time: it holds an exclusive lock on
`data/.scheduler.lock`. A second `just daemon` exits before opening a browser.

## Storage retention

The raw response tree grows quickly, especially `data/MarketplacePDPContainerQuery`
(multiple GB is normal). A cycle therefore:

- runs the PDP dedupe (keeps the newest response per listing),
- moves processed search responses older than `-archive-after` (default 7 days)
  from `data/<Query>/` to `data/archived/<Query>/`,
- deletes archived files older than `-keep-archived` (default 30 days).

Detail records (`data/__Details/`) and PDP responses are never archived. Run the
same maintenance manually with:

```bash
just archive
just archive -archive-after=24h -keep-archived=720h
```

## Logs

The systemd unit passes `-log-file=data/scheduler.log`, so logs go to both the
journal and that file:

```bash
just service-logs                          # journalctl --user -u fme-schedule
tail -f data/scheduler.log                 # file copy
journalctl --user -u fme-schedule --since "1 hour ago"
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-terms` | `search-terms.txt` | Terms file |
| `-interval` | `6h` | Pause between cycle starts |
| `-per-term-delay` | `10m` | Pause between consecutive searches |
| `-scroll-seconds` | `240` | Max scroll wait per search (0 disables scrolling) |
| `-inline-descriptions` | `true` | Pull new descriptions during each search (disable with `=false`) |
| `-desc-queue-limit` | `50` | Max listing ids waiting for a description pull |
| `-schedule-delay-ms` | `2000` | Delay between description requests |
| `-rate-limit-wait` | `300` | Initial wait before retrying a rate-limited PDP request |
| `-max-rate-limit-wait` | `1800` | Max total wait for the rate limit to clear per run |
| `-archive-after` | `168h` | Age before processed responses are archived |
| `-keep-archived` | `720h` | Age before archived responses are deleted |
| `-once` | `false` | Run one cycle now and exit (a current block is still honoured) |
| `-log-file` | empty | Also append logs to this file |
| `-headless` | auto | Force headless; by default a display is used when available |

## Troubleshooting

- **Service exits immediately** — run `just daemon -once` in the foreground; a
  missing `search-terms.txt` or `.env` is printed there.
- **"open session: ensure logged in"** — the copied session expired. Log into
  Facebook in normal Chrome, then `just sync-profile`; the next cycle refreshes
  the copy automatically.
- **Keyring / decryption errors in the journal** — the user service started
  without the unlocked keyring. Log into the desktop session and restart the
  service; do not enable lingering unless you handle keyring unlock at boot.
- **"another fme scheduler may be running"** — a second instance, or a stale
  process still holding `data/.scheduler.lock`. Check `pgrep -af fme`.
- **Chrome fails to launch** — another browser is using the fme profile
  (`~/.local/share/fme-chrome`). The scheduler uses its own copy, so close any
  manual `just run`/`just search` process first.
- **Nothing happens for hours** — `just status` shows `blocked until`; a
  throttled account is waiting out its backoff. Do not restart it repeatedly.
