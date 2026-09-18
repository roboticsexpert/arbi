# Chain history

Added 2026-09-13. Every arbitrage chain's profit percentage is logged per minute
so the dashboard can chart a route **and its reverse** over time.

## What is stored

The finder recalculates every 10s. `internal/history` hooks into it with
`Finder.OnChains` and folds each run into an in-memory bucket per chain path;
when the minute rolls over the bucket is written to SQLite:

| column | meaning |
|---|---|
| `minute` | unix seconds, start of the minute (UTC) |
| `samples` | finder runs folded into the minute (normally 6) |
| `avg` / `min` / `max` / `last` | profit % **with** fees |
| `avg_no_fee` | profit % without fees |

Paths are interned in `chain_paths` (`id`, `path`, `first_seen`, `last_seen`) so
the minute table stores an integer, not the ~70-byte path string.

- A minute is only written once it completes, so the newest point is up to a
  minute behind the live chain cards.
- A chain that did not exist in a minute (no liquidity, source down) has no row.
  The chart shows that as a gap, not an interpolated line.
- On SIGTERM the partial minute is flushed; if the process restarts within the
  same minute the rows merge (sample-weighted avg, min of mins, max of maxes).
  During a Swarm `start-first` rollover two containers briefly write the same
  DB, which merges the same way — the overlap minute just has extra samples.
- Size: ~80 bytes per row on disk. 20 paths × 30 days ≈ 70 MB.

## Configuration

| variable | default | |
|---|---|---|
| `HISTORY_DB_PATH` | `data/history.db` | relative to the working dir (`/home/appuser` in the image) |
| `HISTORY_RETENTION_DAYS` | `30` | rows older than this are pruned hourly; `0` keeps everything |

If the database cannot be opened (typically an unwritable volume) the backend
logs `[History] disabled - cannot open …` and keeps running; the history
endpoints return **503** and the dashboard panel says history is disabled.

The driver is `modernc.org/sqlite` (pure Go), because the Dockerfile builds with
`CGO_ENABLED=0`. It is pinned to **v1.40.1**: newer versions require Go 1.25 and
the image builds on `golang:1.24`.

### Persistence

Without a volume every redeploy starts with an empty history.

- **Docker Swarm** — `docker-compose.yml` mounts the named volume `arbi-history`
  at `/home/appuser/data`. The Dockerfile creates that directory owned by
  `appuser`, so a fresh named volume inherits the right ownership.
- **Railway** — attach a volume to the `arbi` service mounted at
  `/home/appuser/data`. Railway volumes are mounted as root and the image runs
  as `appuser`, so also set `RAILWAY_RUN_UID=0`, otherwise the DB can't be
  created and history is disabled. Done 2026-09-13 — volume `arbi-volume`, see [railway-deployment.md](railway-deployment.md).

## API

Both behind `DASHBOARD_TOKEN`.

`GET /history/paths` → `[{path, first_seen, last_seen}]`, most recently seen first.

`GET /history?path=<p>&path=<p2>&from=<unix>&to=<unix>&step=<minutes>`

- `path` repeatable, 1–10. The dashboard always asks for a Go/Return pair.
- `from`/`to` default to the last 24h.
- `step` ∈ `1, 5, 15, 60, 240, 1440`; omitted = the smallest step that keeps a
  series ≤ 3000 points: 24h → 1 min, 7d → 5 min, 30d → 15 min. Coarser buckets
  are sample-weighted averages with the min/max across the bucket.

```json
{"from": 1789305244, "to": 1789308844, "step": 1,
 "series": {"IRT-ecogold-…-IRT": [{"t": 1789305300, "avg": 0.0106, "min": -0.0035,
   "max": 0.0281, "last": 0.0281, "avg_no_fee": 0.4106, "samples": 6}]}}
```

## Dashboard

`frontend/src/components/ChainHistory.tsx`, drawn with
[lightweight-charts](https://github.com/tradingview/lightweight-charts) v5.

- Route picker lists live pairs first (●, same order as the chain cards), then
  pairs that only exist in history (○) by last seen. A card's **History chart**
  button selects its pair and scrolls to the panel.
- **Go** (blue `#3987e5`) and **Return** (orange `#d95926`) are the bucket
  averages; **Round trip** (dashed gray) is `(1+go)(1+return)−1` at the same
  minute, computed client-side from the averages — an approximation of the
  cost of opening along Go and closing along Return in that minute. Colours are
  slots 1–2 of the dataviz reference palette, validated against the dashboard's
  dark surface.
- Ranges 1H / 6H / 24H / 7D / 30D, with-fee / no-fee toggle, crosshair tooltip
  (avg plus min … max per leg), and a table view of the newest 240 buckets.
- Refreshes every 60s. The chart only refits when the pair or range changes, so
  a zoom survives a refresh.

Two lightweight-charts behaviours had to be worked around:

1. The time scale is **index-based**: a missing bucket takes no space. Series
   are laid on the full `[from, to)` bucket grid with whitespace entries, or a
   2-hour outage would collapse into one bar.
2. A line is still **drawn across whitespace**. A segment takes the colour of the
   point it starts from, so the last point before a gap is made transparent.
