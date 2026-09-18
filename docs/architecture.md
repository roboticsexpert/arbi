# Arbi — repository layout

Monorepo. Two independently deployable services, one git repo.

```
Arbi/
├── backend/            Go service: price sources, arbitrage finder, REST API
│   ├── main.go         routes + handlers
│   ├── internal/
│   │   ├── arbitrage/  chain finder, calculator, path helpers
│   │   ├── backtest/   replays history as positions (cost model + engine)
│   │   ├── config/     env loading (.env is defaults only, real env wins)
│   │   ├── history/    per-minute chain profit log (SQLite)
│   │   ├── httpx/      CORS + dashboard token gate
│   │   ├── metrics/    Prometheus gauges + balance snapshot store
│   │   ├── orderbook/  store, types, source interface
│   │   └── price-sources/  kucoin, nobitex, ecogold, binance
│   ├── docs/           generated swagger (Go package `arbi/docs`)
│   ├── Dockerfile
│   └── railway.json
├── frontend/           React + Vite + TS + Tailwind v4 dashboard
│   ├── src/
│   │   ├── lib/        api client, formatting
│   │   └── components/ chains, order books, balances, token gate
│   ├── Caddyfile       static server, listens on $PORT
│   ├── docker-entrypoint.sh   writes /srv/config.js from $API_URL
│   ├── Dockerfile
│   └── railway.json
├── docs/               project documentation (this folder)
├── docker-compose.yml  Docker Swarm stack (GitLab CI deploy)
└── .gitlab-ci.yml      builds both images, deploys the stack
```

## Backend API

| Endpoint | Auth | Purpose |
|---|---|---|
| `GET /up` | open | health check (Railway + Swarm) |
| `GET /metrics` | open | Prometheus scrape |
| `GET /swagger/*` | open | API docs |
| `GET /overview` | token | everything the dashboard needs in one payload |
| `GET /orderbooks` | token | all order books |
| `GET /orderbooks/:source[/:base/:quote]` | token | filtered order books |
| `GET /stats` | token | store statistics |
| `GET /arbitrage` | token | latest chains |
| `GET /balances` | token | wallet balances per source |
| `GET /history/paths` | token | chain paths with recorded history |
| `GET /history?path=…&path=…` | token | profit % per path in minute buckets — see [chain-history.md](chain-history.md) |
| `GET /backtest` | token | replays history as positions: entries, exits, P&L — see [backtest.md](backtest.md) |

`/overview` exists so the dashboard makes one request per poll instead of four.

### Auth

Data endpoints are gated by `DASHBOARD_TOKEN`. The token is sent as
`X-Dashboard-Token: <token>`, or `?token=<token>` for curl and browser tabs.
**When `DASHBOARD_TOKEN` is unset the gate is disabled entirely** and every
endpoint is open, so existing unauthenticated consumers keep working.

`/up` and `/metrics` are deliberately outside the gate — the platform health
check and Prometheus must not need a secret.

### CORS

`DASHBOARD_ORIGINS` is a comma-separated allowlist of browser origins (`*`
allows any). The dashboard runs on its own domain, so without this the browser
blocks every API call.

## Frontend

Polls `GET /overview` every 2 seconds. There is no WebSocket: the backend
recalculates arbitrage on a 10s timer, so a 2s poll is already finer-grained
than the data changes, and it has no reconnect logic to get wrong.

Live-ness is made visible rather than assumed:
- connection pill (Live / Connecting / Disconnected) with time since last success
- per-market age with a colour change once a book is older than 60s
- green/red flash on a best bid or ask that moved since the previous poll
- explicit "no data from <source>" banner for a registered source with 0 books

### Chains are shown as Go / Return pairs

The finder's DFS emits both directions of every loop as separate chains. A
position is opened along one direction and closed along the other, so the
dashboard groups them (`frontend/src/lib/pairs.ts`); the backend API is
unchanged and still returns a flat list.

- **Matching** uses the `path` string. Paths alternate asset and venue
  (`IRT-ecogold-GOLD18-convert-PAXG-kucoin-USDT-nobitex-IRT`) and asset symbols
  never contain `-`, so reversing the tokens gives exactly the reverse route's
  path. The same rule pairs live chains and paths that only exist in history,
  which is why it is path-based rather than step-based.
- **Which leg is "Go"** is fixed by the route (lexicographically smaller path),
  not by which leg is currently better, so labels don't swap between polls and
  the chain cards and the history chart always agree. The currently better leg
  carries an `OPEN` marker when profitable.
- **Round trip** = `(go.end/start) × (return.end/start) − 1`: the result of
  opening and immediately closing at current prices — effectively the spread
  cost of the pair. Shown with and without fees.
- A chain whose other direction is missing (one book has no liquidity on the
  needed side) is still shown, with that row marked not available.
- Each expanded card has a **History chart** button that selects the pair in
  the Chain history panel.

The **Backtest** panel below the history chart replays those same recorded
series as positions — entries, exits, and profit or loss under a chosen cost
structure. See [backtest.md](backtest.md).
- Pairs are sorted by their best leg's profit. The header tile counts pairs.

The API base URL is **not** baked into the bundle. `docker-entrypoint.sh` writes
`/srv/config.js` from the `API_URL` variable at container start, so one image can
point at any backend. It falls back to `VITE_API_URL` (dev) then to the page
origin.

The access token is held in `localStorage` only — it is never in the URL, and
"Lock" clears it.
