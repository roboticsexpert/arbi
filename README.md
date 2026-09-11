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

## Deployment

- **Railway** (both services) — [docs/railway-deployment.md](docs/railway-deployment.md)
- **Docker Swarm via GitLab CI** — `.gitlab-ci.yml` + `docker-compose.yml`
