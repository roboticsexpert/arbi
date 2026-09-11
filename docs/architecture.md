# Arbi — repository layout

Monorepo. Two independently deployable services, one git repo.

```
Arbi/
├── backend/            Go service: price sources, arbitrage finder, REST API
│   ├── main.go         routes + handlers
│   ├── internal/
│   │   ├── arbitrage/  chain finder and calculator
│   │   ├── config/     env loading (.env is defaults only, real env wins)
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

The API base URL is **not** baked into the bundle. `docker-entrypoint.sh` writes
`/srv/config.js` from the `API_URL` variable at container start, so one image can
point at any backend. It falls back to `VITE_API_URL` (dev) then to the page
origin.

The access token is held in `localStorage` only — it is never in the URL, and
"Lock" clears it.
