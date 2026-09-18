# MetaTrader 5 Integration

Reads live metal quotes from the MetaTrader 5 account
`3242457` on `InternationalTrading-Server` (International Trading Brachium Ltd.)
and exposes them as a normal price source alongside KuCoin, Nobitex and EcoGold.

## Why it works backwards from every other source

Every other source in this project dials out: we open a WebSocket or poll a REST
endpoint. MetaTrader cannot be used that way. **MT5 has no server-side API a
backend can call** — quotes only exist inside a running terminal. Specifically:

- **MT5 Web API / Manager API** is broker-side. It requires an MT5 *server*
  licence and manager credentials. Not obtainable as a client of the broker.
- **The MT5 WebTerminal** speaks an undocumented proprietary binary protocol
  over a WebSocket gateway. Scraping it is fragile and against the MQL5 ToS.
- **TradingView's MetaTrader integration** is a trading panel, not a data
  export. TradingView has no outbound API for a connected account's quotes, and
  its charts show its own aggregated feed rather than the broker's prices.

So the terminal dials out to us instead. An Expert Advisor
([`mt5/ArbiPriceFeed.mq5`](../mt5/ArbiPriceFeed.mq5)) reads top-of-book and
POSTs it to `/api/mt5/ticks`.

The EA is just *one* producer. The Go side
([`internal/price-sources/mt5`](../backend/internal/price-sources/mt5)) is a
plain sink, so replacing the EA with a hosted bridge (MetaApi) or a headless
terminal on a VPS later changes nothing behind the endpoint.

## Symbol mapping

The broker quotes gold in grams, kilograms and ounces. The mapping lives in
[`symbols.go`](../backend/internal/price-sources/mt5/symbols.go):

| MT5 symbol      | Pair         | Divisor | Meaning                        |
|-----------------|--------------|---------|--------------------------------|
| `GOLD_kilogram` | `GOLD24-USD` | 1000    | 1 gram of 24k gold, in USD     |
| `GOLD_ounce`    | `XAU-USD`    | 1       | 1 troy ounce of gold, in USD   |
| `SILVER_ounce`  | `XAG-USD`    | 1       | 1 troy ounce of silver, in USD |

### Why not `GOLD_gram`

`GOLD_gram` is the obvious choice and it is the wrong one. The broker gives that
symbol only 2 decimal digits, which rounds the spread away completely — both
sides print the same number:

```
GOLD_gram       141.21   / 141.21     spread 0.00     <- spread lost
GOLD_kilogram   141209   / 141212     spread 0.0021%
GOLD_ounce      4392.12  / 4392.20    spread 0.0018%
```

A zero spread reads as a free lunch to the arbitrage finder. `GOLD_kilogram`
carries the identical price with enough precision to keep the spread, so we take
that and divide by 1000. `GOLD_gram` is deliberately absent from the map.

## Setup

### 1. Backend

```
MT5_INGEST_TOKEN=<a long random string>
MT5_STALE_SECONDS=90
```

`/api/mt5/ticks` is the only write endpoint in the API and is **only registered
when `MT5_INGEST_TOKEN` is set**. An open ingest would let anyone inject gold
prices into the arbitrage graph, so an unset token disables the route entirely
rather than leaving it unauthenticated.

### 2. Terminal

1. **Tools → Options → Expert Advisors** → tick *Allow WebRequest for listed
   URL* and add the backend origin (`http://127.0.0.1:8080` locally).
2. Enable **Algo Trading** in the toolbar.
3. Copy `mt5/ArbiPriceFeed.mq5` into `MQL5/Experts/` in the terminal's data
   folder (File → Open Data Folder), compile it in MetaEditor (F7).
4. Drag the EA onto any chart and set `InpToken` to `MT5_INGEST_TOKEN`.

Under Wine on macOS, `127.0.0.1` reaches the host loopback, so a backend running
natively on the Mac is reachable. If it is not, use the Mac's LAN IP in both the
EA endpoint and the WebRequest whitelist.

### 3. Verify

```bash
curl -s localhost:8080/orderbooks/mt5 | python3 -m json.tool
```

## Staleness

Metals CFDs close over the weekend, and the EA only runs while the terminal is
up. A quote with no fresh tick for `MT5_STALE_SECONDS` has its **price levels
emptied** — the pair stays visible with its last timestamp, but carries no bids
or asks, so `buildGraph` stops emitting edges for it.

This matters because the central store keeps the last book it was handed.
Withholding updates would leave a frozen Friday gold price looking live to the
arbitrage finder for as long as the terminal stayed down.

Staleness is measured against the **broker's tick time**, not our receive time.
The EA heartbeats every 30s even when nothing moves, so receive time would make
a frozen weekend quote look perpetually fresh. A tick that arrives already older
than the window is logged, which is the signal for a closed market or a skewed
broker clock.

## Deployment

The Go side deploys like any other change. The part that does **not** move with
it is the feed: the Expert Advisor runs in a terminal, so deploying the backend
does not make gold 24/5. It makes the *sink* always-on, and the EA pushes over
the internet instead of loopback. For an always-on feed, migrate the EA to MQL5
Virtual Hosting (see *Running 24/5 on MQL5 Virtual Hosting* below).

### 1. Set the token on the target

Railway (see [railway-deployment.md](railway-deployment.md)):

```bash
railway variable set MT5_INGEST_TOKEN=<long random string> --service arbi
```

Docker Swarm via GitLab CI: add `MT5_INGEST_TOKEN` as a **masked** CI/CD
variable. `docker-compose.yml` already passes it through to the `web` service.

The token is deliberately **not** in `.env.production` — the Dockerfile bakes
that file into the image, which is pushed to the registry.

Without the variable the route is not registered and the EA gets a 404 on every
push, which is the intended failure mode: an open ingest would let anyone inject
gold prices into the arbitrage graph.

### 2. Point the EA at the public URL

In the EA inputs set `InpEndpoint` to
`https://<backend-host>/api/mt5/ticks`, and add the **origin** (scheme + host,
no path) to *Tools → Options → Expert Advisors → Allow WebRequest for listed
URL*. The whitelist matches on origin, so `http://127.0.0.1:8080` does not cover
the production host — add both if you want to switch between them.

### 3. Which backend to feed

There are two deployments and they are not equivalent for this chain:

- **Docker Swarm (Iran-hosted)** — Nobitex works, so `USDT/IRT` is present and
  the `GOLD24 → GOLD18 → IRT` loop closes.
- **Railway** — Nobitex is intermittently blocked from non-Iranian datacentre
  ranges. Without the `IRT ↔ USDT` edge no chain closes at all, so MT5 shows as
  a price but produces nothing.

To feed both, attach the EA to two charts with different `InpEndpoint` and
`InpToken` values; each instance pushes independently.

### 4. Operational notes

- **Restarts.** Orderbooks are in memory, so a redeploy empties the MT5 book.
  The EA's 30s heartbeat refills it within one interval — no action needed.
- **Latency.** A push from the Mac to a remote host adds RTT to every tick. At a
  1s cadence with a 90s staleness window this has plenty of headroom.
- **The token is the only control on this endpoint.** It is a bearer secret sent
  in `X-MT5-Token` over HTTPS, and it is visible in the EA's properties dialog on
  the machine running the terminal. Use a long random value and rotate it in
  both places together.

## Running 24/5 on MQL5 Virtual Hosting

Acquired 2026-09-11 to take the feed off the Mac. **MQL5 Virtual Hosting is not
a machine you log into.** There is no RDP, SSH or file access — you configure
the EA in the local terminal and *migrate* it. So nothing (including Claude)
can operate the VPS directly; the only way to observe it is the VPS journal in
the terminal and the backend's `/orderbooks/mt5`.

What makes `ArbiPriceFeed` compatible: it uses no DLLs (forbidden on the
virtual host — the program is killed on the first DLL call) and `WebRequest`
is allowed as long as the URL whitelist is set before migrating.

What migrates: charts with running EAs **and their inputs** (so the token goes
with it), compiled `.ex5` files, and the WebRequest permission + allowed-URL
list. Free plans take 16 charts, paid 32; the feed needs one per backend.

### Procedure

1. Backend must be reachable from the internet with `MT5_INGEST_TOKEN` set.
   `127.0.0.1` means nothing on the VPS — use the public HTTPS URL.
2. Local terminal: whitelist the public origin, attach the EA to a chart with
   `InpEndpoint` = public URL and `InpToken` set, confirm `[Arbi]` logs show
   no errors and `/orderbooks/mt5` fills.
3. Navigator → Accounts → right-click the VPS → **Synchronize → Experts only**
   (or *All*). Account login must not use one-time passwords.
4. Check **VPS → Journal** for `[Arbi] Feeding N symbol(s)`.
5. Remove the EA from the local chart (or close the terminal), otherwise both
   push. Duplicate pushes are harmless but double the traffic and hide a dead
   VPS behind a live Mac.

**Migration is a snapshot.** Changing inputs or recompiling locally does not
reach the VPS until you synchronize again.

### Open questions

- Whether the MetaQuotes datacentre (picked near the broker server, not Iran)
  can reach the Iran-hosted Swarm deployment. Railway is reachable but cannot
  close a chain (see *Which backend to feed*).

### State as of 2026-09-11

Earlier that day Railway was running a build older than the MT5 source and
`POST /api/mt5/ticks` returned 404. A *Redeploy* from the Railway dashboard
does **not** fix this — it rebuilds the last uploaded snapshot, not new code.

Fixed by `railway up` from `backend/` (commit `3459ab9`) with
`MT5_INGEST_TOKEN` set. Verified after deploy: the untokenised ingest returns
**401** (route registered), `/stats` reports 4 sources including `mt5`.

Feed confirmed live at 13:51 UTC: the EA pushes `200 POST /api/mt5/ticks`
roughly every second from `194.61.89.84`, and the finder is closing chains
through the MT5 leg on Railway (Nobitex happened to be reachable), e.g.
`IRT-ecogold-GOLD18-convert-GOLD24-mt5-USD-convert-USDT-nobitex-IRT`.

## Arbitrage wiring

The MT5 leg is connected to the graph by four conversions in
[`finder.go`](../backend/internal/arbitrage/finder.go):

```go
{From: "GOLD24", To: "GOLD18", Rate: gold24ToGold18},
{From: "GOLD18", To: "GOLD24", Rate: 1.0 / gold24ToGold18},
{From: "USD",    To: "USDT",   Rate: 1},
{From: "USDT",   To: "USD",    Rate: 1},
```

`GOLD24 → GOLD18` alone is not enough to close a loop: the MT5 book is quoted
in `USD` and nothing else in the graph touches `USD`, so the `USD ↔ USDT` pair
is what makes the leg reachable from `IRT`.

### The two constants

`gold24ToGold18` is **derived from `paxgToGold18`**, not computed from first
principles:

```go
gramsPerTroyOunce = 31.1035
paxgToGold18      = 41.4665196
gold24ToGold18    = paxgToGold18 / gramsPerTroyOunce  // 1.3331785683
```

Computing it independently as `31.1034768 / 0.750 = 41.4713` would disagree with
the PAXG factor already in the file by ~0.011% — that factor implies a purity of
0.7500874 rather than a clean 0.750. Two routes to the same metal that disagree
produce a permanent phantom edge between the MT5 leg and the PAXG leg, so both
are pinned to the same constant.

`USD ↔ USDT` is pinned at **1**. This is an assumption, not a quote: any real
USD/USDT basis is silently absorbed into the reported profit.

### What the resulting chains actually claim

A typical chain looks like:

```
IRT -ecogold-> GOLD18 -convert-> GOLD24 -mt5-> USD -convert-> USDT -nobitex-> IRT
```

Read literally, that says: buy physical 18k gold from EcoGold with rials, then
**sell it on MetaTrader**. That second step is not the same trade. An MT5
position is a CFD — going short does not dispose of physical metal, and it
settles as USD in a broker account rather than as USDT in a wallet.

So these numbers are a **signal, not an executable route**: they measure where
EcoGold's gold price sits relative to world spot, expressed in rials. Treating
the percentage as realisable profit requires a way to move value between the
broker account and the Iranian legs, which the graph does not model.

Still unmodelled, and needed before the numbers mean money:

- **Commission and swap.** `exchangeFees[mt5]` is a placeholder 0.1%. The
  broker's spread is already inside the bid/ask, but commission per lot and
  overnight swap are not. Both need the symbol specification.
- **Contract size and lot limits.** `Volume` in `symbols.go` is a placeholder,
  so chain sizing is fictional.
- **USD/USDT basis**, pinned at 1 above.
- **Settlement between the broker account and the IRT legs.**

## Files

| Path | Role |
|---|---|
| `mt5/ArbiPriceFeed.mq5` | Expert Advisor; reads ticks, POSTs them |
| `backend/internal/price-sources/mt5/source.go` | `PriceSource` implementation, staleness |
| `backend/internal/price-sources/mt5/symbols.go` | MT5 symbol → pair mapping |
| `backend/main.go` | `postMT5Ticks` handler, route registration |
| `backend/internal/httpx/middleware.go` | `RequireTokenHeader`, `X-MT5-Token` |
