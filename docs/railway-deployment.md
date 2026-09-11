# Railway Deployment

Deployed 2026-09-08. Runs alongside the existing GitLab CI → Docker Swarm setup
(`.gitlab-ci.yml` + `docker-compose.yml`), which is unchanged.

## Resources

| | |
|---|---|
| Workspace | Mahdi Youseftabar's Projects |
| Project | `arbi` — `2093829c-d63e-45b0-aa1c-edd8eb0d5665` |
| Environment | `production` — `add21eb4-ccae-45cf-a39a-d903784ad820` |
| Backend service | `arbi` — `3001fa77-af3e-4184-a1fc-6d24d4e7fa17` |
| Backend URL | https://arbi-production-a97a.up.railway.app |
| Dashboard service | `arbi-dashboard` — `586a1e4d-6804-48a3-a4fd-cffb3192319b` |
| Dashboard URL | https://arbi-dashboard-production.up.railway.app |

Dashboard: https://railway.com/project/2093829c-d63e-45b0-aa1c-edd8eb0d5665

## How it deploys

The git remote is self-hosted GitLab (`gitlab.zisef.ir`), so Railway's GitHub
integration is not available. Both services are pushed from a working copy.

**Each service directory is linked to its own Railway service.** This matters:
`railway up` uploads the *linked directory*, not the current one. With only the
repo root linked, running `railway up` from `backend/` silently uploaded the
whole repo, Railway rebuilt the previous cached image, and the deploy failed with
no runtime logs. The links are already set up:

```
Arbi/backend   -> service arbi
Arbi/frontend  -> service arbi-dashboard
```

If they are ever lost (new machine, cleared `~/.railway/config.json`), restore
them with:

```bash
cd backend  && railway link --project arbi --environment production --service arbi
cd frontend && railway link --project arbi --environment production --service arbi-dashboard
```

Then deploy from inside the directory you want to ship:

```bash
cd backend  && railway up --ci -m "<message>"   # exit 0 = deployed
cd frontend && railway up --ci -m "<message>"
```

`--ci` streams the build and exits non-zero on failure, which is why it is
preferred over `--detach` here. With `--detach`, poll instead:

```bash
railway deployment list --service <name> --json   # wait for status SUCCESS
```

Each service has its own `railway.json` selecting the `DOCKERFILE` builder. The
backend adds a `/up` health check; the dashboard has none (Caddy serves static
files and is up as soon as it binds).

Railway injects `PORT`. Gin's `router.Run()` with no argument already honours it,
and the Caddyfile listens on `:{$PORT:8080}`, so neither service needed a port
change.

## Variables

**`arbi` (backend)** — secrets copied from local `backend/.env`:
`NOBITEX_TOKEN`, `ECOGOLD_TOKEN`, `ECOGOLD_PASSWORD`, `KUCOIN_DEFAULT_SYMBOLS`,
`BINANCE_DEFAULT_SYMBOLS`, `NOBITEX_DEFAULT_SYMBOLS`, `GIN_MODE=release`, plus:

- `DASHBOARD_TOKEN` — gates every data endpoint. Unset it to make the API fully
  open again.
- `DASHBOARD_ORIGINS` — `https://arbi-dashboard-production.up.railway.app`.
  The dashboard is on a different domain, so the browser needs this to be
  allowed or every request fails CORS.
- `MT5_INGEST_TOKEN` — gates `POST /api/mt5/ticks`, the MetaTrader feed and the
  only write endpoint in the API. **The route is not registered when this is
  unset**, so the EA gets a 404 rather than an open ingest. Not in
  `.env.production`, since that file is baked into the image. See
  [mt5-integration.md](mt5-integration.md).

Note that MT5 alone does not produce arbitrage chains on Railway: the
`GOLD24 → GOLD18 → IRT` loop needs the `IRT ↔ USDT` edge, which comes from
Nobitex — unreliable here (see below).

**`arbi-dashboard` (frontend)**:

- `API_URL` — `https://arbi-production-a97a.up.railway.app`. Read at container
  start and written into `/srv/config.js`, so changing it needs a restart, not
  a rebuild.

`PROXY_URI` is deliberately **not** set on Railway — it does not need a proxy to
reach KuCoin, and the local value (`http://127.0.0.1:2081`) does not exist there.

Rotating the dashboard token:

```bash
railway variable set DASHBOARD_TOKEN=<new> --service arbi
```

## Code changes required to run on Railway

1. **`internal/config/variable.go`** — `init()` used to `os.Setenv()` every key
   from `.env` unconditionally. The Dockerfile bakes `.env.production` in as
   `.env`, so that file silently overrode any variable set in the Railway
   dashboard. It now only fills in keys that are not already present in the
   environment, so platform variables win and `.env` acts as defaults.
2. **`internal/price-sources/kucoin/client.go`** — the transport's proxy function
   returned `url.Parse(config.PROXY_URI)`. With `PROXY_URI` empty that returns a
   non-nil but empty `*url.URL`, which makes Go's transport attempt a proxy
   connection to nothing. It now returns `nil, nil` when `PROXY_URI` is empty
   (direct connection).
3. **`internal/httpx/`** (new) — CORS allowlist and the `DASHBOARD_TOKEN` gate,
   added so the dashboard can call the API from another domain without exposing
   balances and arbitrage chains publicly.
4. **`internal/metrics/balances.go`** (new) — balances previously lived only in
   Prometheus gauges, which cannot be read back. A snapshot store now mirrors
   them so `GET /balances` and `/overview` can serve them.

## Source status from Railway (verified in-container via `railway ssh`)

| Source | Status |
|---|---|
| KuCoin | Working — `PAXG-USDT`, `PAXG-BTC` streaming |
| EcoGold prices | Working — `backend.ecogold.ir` returns 200 |
| Nobitex | **Blocked** — see below |

### Nobitex connectivity is intermittent

On the first deploy (2026-09-08 ~08:01 UTC) Nobitex was completely unreachable
from the container, verified with `railway ssh`:

- `apiv2.nobitex.ir` (REST, wallet balances) — connection timed out entirely.
- `ws.nobitex.ir` (order book feed) — TLS connected (307 on plain HTTPS) but the
  WebSocket upgrade failed with `bad handshake` on every retry.

By ~08:16 UTC the same container was streaming `USDT/IRT` and `BTC/IRT` normally
with no configuration change, and the arbitrage finder started closing loops
(+2.18% best chain). Nobitex evidently rate-limits or blocks non-Iranian
datacenter ranges inconsistently rather than outright.

**Treat this as unreliable, not solved.** The dashboard shows a "no data from
nobitex" banner and per-market staleness precisely so this is visible when it
happens again. If it needs to be dependable:

1. Route only Nobitex through an Iran-side proxy. `PROXY_URI` is currently wired
   **only** into the KuCoin and Binance clients — `internal/price-sources/nobitex/`
   would need its own support, and a separate variable (e.g. `NOBITEX_PROXY_URI`)
   is cleaner since the two proxies solve opposite problems.
2. Or keep Nobitex on the Iran-hosted Swarm deployment and treat Railway as the
   KuCoin/EcoGold view.

Without Nobitex the graph has no IRT ↔ crypto edge and no chain can close.

## Unrelated pre-existing problem: both private tokens are expired

`ECOGOLD_TOKEN` and `NOBITEX_TOKEN` in `.env` return 401 **from the local machine
too**, so this is not caused by the deployment:

- EcoGold `POST /api/auth/verify-password` → `401 {"error_code":"UNAUTHORIZED_ERROR"}`
- Nobitex `GET /users/wallets/list` → `401 {"detail":"توکن غیر مجاز"}`

Balance metrics (`UpdateWalletBalanceMetrics`) are therefore empty everywhere.
Refresh both tokens, update `.env`, then re-push with
`railway variable set KEY=value --service arbi`.
