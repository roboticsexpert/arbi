# Arbi

Cross-venue arbitrage monitor. Aggregates order books from KuCoin, Nobitex and
EcoGold, searches for profitable IRT → … → IRT chains, and serves them over a
REST API and a live dashboard.

## Layout

Monorepo with two deployable services — see [docs/architecture.md](docs/architecture.md).

- `backend/` — Go 1.24, Gin. Price sources, arbitrage finder, REST API, Prometheus metrics.
- `frontend/` — React 19 + Vite + Tailwind v4. Live dashboard.

## Running locally

```bash
# backend
cd backend
cp .env.example .env      # fill in tokens
go run .                  # :8080, or set PORT

# dashboard (separate shell)
cd frontend
npm install
cp .env.example .env      # point VITE_API_URL at the backend
npm run dev               # :5173
```

Set `DASHBOARD_TOKEN` on the backend to require a token; leave it unset and the
API is open.

## Docs

- [docs/architecture.md](docs/architecture.md) — repo layout, API, dashboard
- [docs/mt5-integration.md](docs/mt5-integration.md) — MetaTrader feed
- [docs/chain-history.md](docs/chain-history.md) — per-minute profit history
- [docs/trade-economics.md](docs/trade-economics.md) — cost of holding a chain
  1–7 days, and the spread reversion needed to clear a target

## Deployment

- **Railway** (both services) — [docs/railway-deployment.md](docs/railway-deployment.md)
- **Docker Swarm via GitLab CI** — `.gitlab-ci.yml` + `docker-compose.yml`
