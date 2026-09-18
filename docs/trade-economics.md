# Trade economics — holding a chain for 1–7 days

Started 2026-09-18. The finder answers *"is there a spread right now"*. This
document answers the question that comes after it: **if we actually open a
position and carry it for one to seven days, what does it cost, and how far does
the spread have to come back before we clear our target?**

Fee data below was read off the venues on **2026-09-18**. Two inputs are still
missing and both are blocking — see [§7](#7-still-missing).

## 1. The chain is a signal; the position is a pair trade

`mt5-integration.md` already makes this point and it governs everything here. A
chain like

```
IRT -ecogold-> GOLD18 -convert-> GOLD24 -mt5-> USD -convert-> USDT -nobitex-> IRT
```

reads as a sequence of conversions, but you cannot execute it as one. Selling
gold on MT5 opens a CFD short; it does not dispose of the metal you bought at
EcoGold. What you actually put on is **both legs at the same time**, and unwind
both when the spread narrows.

So the finder's round trip `(1+go)·(1+ret) − 1` remains the correct measure of
the spread — but the *cost* of the position is two sets of trading fees (open
and close), plus a per-day carry on every leveraged leg, plus whatever it costs
to move capital in and out of each venue.

## 2. Confirmed fee data

### 2.1 Nobitex spot — `nobitex.ir/pricing`

Charged on **both sides** of every trade. Tier is recalculated daily at
03:00–04:00 from trailing 30-day volume.

| tier | 30-day volume | IRT maker | IRT taker | USDT maker | USDT taker |
|---|---|---|---|---|---|
| **پایه (base)** | < 100M T | **0.25%** | **0.25%** | 0.10% | 0.13% |
| VIP1 | 100–300M T | 0.17% | 0.20% | 0.095% | 0.12% |
| VIP2 | 300M–1B T | 0.15% | 0.19% | 0.09% | 0.11% |
| VIP3 | 1–5B T | 0.125% | 0.175% | 0.08% | 0.10% |
| VIP4 | 5–20B T | 0.10% | 0.155% | 0.07% | 0.10% |
| VIP5 | 20–80B T | 0.09% | 0.145% | 0.065% | 0.095% |
| VIP6 | > 80B T | 0.08% | 0.135% | 0.06% | 0.09% |

`USDT-IRT` is a **toman market**, so it pays the IRT column — **0.25% taker at
base tier**, not the 0.2% currently hard-coded in `finder.go`.

### 2.2 Nobitex تعهدی (margin) — the important one

- **Leverage 1x–5x.**
- Opening and closing trades pay the **normal spot fee** from the table above.
- On top of that, a **rollover fee every 8 hours** (so 3× per day). The rate is
  **not fixed** — Nobitex sets it daily from pool supply and demand, and it is
  shown in the *موقعیت‌های باز* panel. The pricing page's figures are indicative:

  | market | rollover, long | rollover, short |
  |---|---|---|
  | IRT markets | 0.14% | 0.05% |
  | USDT markets | 0.03% | 0.05% |

- **Rollover is free while the lending pool still has spare capacity.** It only
  starts being charged once the pool is exhausted.
- It is charged on the **borrowed asset value, including unfilled open orders** —
  an order that sits half-filled is billed on its full size.
- **Profit share: 0.5% of the position's profit per extension day**, paid to the
  pool. Day one is exempt, and nothing is taken if the position loses. A 7-day
  hold therefore surrenders 6 × 0.5% = **3% of profit**.
- Max duration is stated as **60 days** on the fee page and **30 days** in the
  FAQ. Either way a 7-day hold is inside the limit.

**Two structural findings from the margin market list:**

- **`XAUT/USDT` has a 5x margin market** (rollover 0.03% long / 0.05% short).
  That is leveraged gold *on the same venue as the IRT leg* — see §4.
- **`USDT/IRT` has no margin market**, and neither does `PAXG`. The rial leg is
  cash-only, which caps the capital multiplier (§3).

### 2.3 Nobitex transfers

- **USDT: only BEP20 (BSC) is active** — withdrawal 0.7 USDT (min 2), deposit
  free (min 0.01). TRC20, ERC20, Polygon, Arbitrum and Optimism are all
  **تعلیق (suspended)**. The earlier assumption of TRC20 was wrong.
- **PAXG (ERC20) deposit and withdrawal: suspended.**
- **XAUT (ERC20) deposit and withdrawal: suspended.**
- IRT deposit: 0.02% (حساب‌به‌حساب / کارت‌به‌کارت), 0% (مستقیم); شتابی disabled.
- IRT withdrawal: 0.02%, min 4,000 T, max 20,000 T per 100M T.

That PAXG and XAUT cannot be moved in or out of Nobitex kills any route that
needs to settle physical gold tokens between KuCoin and Nobitex. Those chains
stay valid as *price signals*; they are not settlement paths.

### 2.4 MT5 — International Trading Brachium

- **Spread-only pricing, "from 1.0 pip", no commission.** The spread is already
  inside the bid/ask the EA pushes us, so charging it again in `exchangeFees`
  double-counts. **The MT5 per-trade fee should be 0**, not the current 0.1%
  placeholder. (Observed spreads are ~0.0018–0.0023%, consistent with 1 pip.)
- **Leverage 1:100 available.**
- **Swap is still unknown** and is not the same thing as commission — see §7.

### 2.5 EcoGold

- Fee **0%**, per your instruction. Any real cost sits in the quoted bid/ask.

### 2.6 KuCoin

Could not be read: the public site only publishes VIP5–VIP12, and LV0–VIP4 are
behind a login. For reference, the published upper tiers are Class A
0.035%/0.055% at VIP5 down to 0%/0.025% at VIP12, with Class B = 2× and
Class C = 3× Class A, and a 20% KCS taker discount.

Working assumption remains **LV0 Class A = 0.1% maker / 0.1% taker**, which is
what `finder.go` already uses. Confirm it from the fee page inside your own
account — that also shows which class `PAXG-USDT` falls into.

This may not matter: with PAXG withdrawal suspended at Nobitex (§2.3), KuCoin
has no settlement path into the rial legs.

## 3. What leverage actually buys here

Per leg *i*, notional `Nᵢ` and margin requirement `mᵢ` (`mᵢ = 1` for cash). In a
matched pair trade every leg carries the same notional `N`, so equity deployed
is `E = N · Σmᵢ` and the **capital multiplier** is

```
K = N / E = 1 / Σ mᵢ
```

`K` scales a move in the spread into return on equity.

| structure | Σm | K |
|---|---|---|
| EcoGold cash + Nobitex `XAUT/USDT` short at 5x | 1 + 0.20 = 1.20 | **0.83** |
| EcoGold cash + MT5 short at 1:100 | 1 + 0.01 = 1.01 | **0.99** |
| both legs cash | 2.00 | 0.50 |

**Leverage is worth at most 2× here, not 100×.** The EcoGold leg is real gold
bought with rials and cannot be leveraged, so it pins `Σm ≥ 1` and caps `K` at
1.0 however much leverage the short side offers. Going from 5x to 1:100 moves
`K` from 0.83 to 0.99 and buys nothing else but a thinner liquidation buffer.

And the effect runs both ways: carry is charged on `N` while profit is measured
against `E`, so **carry as a percentage of equity is `K × Σcᵢ`** — the same
factor that magnifies profit magnifies the financing bill.

## 4. Two executable structures

### Structure A — Nobitex-internal, no cross-border settlement

```
long   EcoGold GOLD18, paid in IRT           cash, m = 1
short  Nobitex XAUT/USDT تعهدی, up to 5x     m = 0.20
fund   Nobitex USDT-IRT spot                 cash
```

Everything settles inside Iran. No broker transfer, no suspended-network
problem. Costs per round trip:

| | |
|---|---|
| EcoGold, both sides | 0% |
| `XAUT/USDT` taker, both sides | 2 × 0.13% = 0.26% |
| `USDT-IRT` taker to fund and unwind | 2 × 0.25% = 0.50% |
| IRT in/out | ~0.04% |
| **fixed total** | **~0.80%** |
| rollover carry, short XAUT | 0.05% per 8h → **0.15%/day** (0 while pool has capacity) |
| profit share | 0.5% of profit per extension day |

`K = 0.83`.

### Structure B — MT5 short

```
long   EcoGold GOLD18, paid in IRT     cash, m = 1
short  MT5 GOLD_kilogram at 1:100      m = 0.01
```

`K = 0.99`, and trading fees are ~0 on both legs (EcoGold 0%, MT5 spread-only).
Its entire cost sits in the two unknowns: **swap per night**, and **what it
costs to get USD in and out of the broker account**.

**The comparison reduces to one question.** Structure A costs ~0.80% fixed plus
up to 1.05% of carry over 7 days. Structure B costs ~0 fixed plus swap plus
broker funding. If the broker round-trip funding cost is below roughly **1.3%**,
B wins; above it, A wins. If capital is *parked* at the broker rather than moved
per trade, that cost amortises away and B wins clearly.

## 5. The breakeven formula

| | |
|---|---|
| `g` | Go-leg result at open, %, net of that leg's trading fees (`chain.profit_percent`) |
| `r` | Return-leg result at close, same basis |
| `RT` | round trip = `(1+g)(1+r) − 1` — what `roundTripPercent()` already computes |
| `T` | profit target on equity, assumed **1%** |
| `D` | days held, 1 … 7 |
| `cᵢ` | daily carry of leg *i*, fraction of notional |
| `X` | one-off transfer costs, fraction of notional |
| `K` | capital multiplier, `1 / Σmᵢ` |
| `p` | profit share per extension day (Nobitex margin: 0.005) |

```
ROE(D) = K · [ RT − (Σcᵢ)·D − X ] · (1 − p·(D−1))
```

Setting `ROE ≥ T`:

```
RT_req(D) = T / [ K · (1 − p·(D−1)) ] + (Σcᵢ)·D + X
```

and the two numbers worth putting on the dashboard:

```
required spread improvement = RT_req(D) − RT_now          (percentage points)
required return leg          r_req = (1 + RT_req(D)) / (1 + g) − 1
```

`RT_now` is normally negative — it is the cost of an immediate round trip — so
the improvement needed is roughly `T/K + carry + transfer + |RT_now|`.

### Worked example — Structure A with the confirmed numbers

Go leg opens at `g = +2.0%`, `RT_now = −1.5%`, `T = 1%`, `K = 0.83`,
rollover 0.15%/day, `X = 0.04%`, `p = 0.5%/day`.

| | D = 1 | D = 7 |
|---|---|---|
| `T / [K·(1−p(D−1))]` | 1.20% | 1.24% |
| carry `Σc·D` | 0.15% | 1.05% |
| transfer `X` | 0.04% | 0.04% |
| **`RT_req`** | **1.39%** | **2.33%** |
| improvement needed vs `RT_now` | **2.89 pp** | **3.83 pp** |
| close when the return leg reaches | `−0.60%` | `+0.32%` |

Read the last row as the trading rule: *opened at +2.0%; if closing on day one,
close once the Return leg prints better than −0.60%; if it drags to day seven
you need the Return leg at +0.32% just to clear 1%.*

**Carry is what makes the week expensive.** Holding seven days instead of one
adds 0.94 pp to the threshold, almost all of it the 0.15%/day rollover. If the
pool has spare capacity the rollover is free and `RT_req(7)` falls to 1.28% —
the *same* trade, roughly 1 pp easier, decided entirely by a number you can only
read on the day. Check it before opening.

## 6. Residual risk the formula does not price

**The FX leg is not automatically hedged.** EcoGold's gold is priced in IRT; the
short settles in USD or USDT. If `USDT/IRT` moves while the position is open,
the long reprices with it and the short does not — a long-gold/short-CFD pair is
implicitly **long rial devaluation**. Over seven days that exposure can easily
exceed the 1% being chased, and `USDT/IRT` has no margin market to close it
cheaply.

**Liquidation.** A 3% adverse gold move over a week is ordinary. At 5x, 20% of
notional is the whole margin; at 1:100 it is 1%. Margin must be held well above
the minimum, and that buffer is idle capital — which is the real argument
against the 1:100 column in §3.

**Rollover is billed on open orders too.** An order that sits partly unfilled is
charged on its full size. Do not leave resting margin orders.

**Weekend gap.** Metals CFDs close Friday and reopen Sunday; the Iranian legs
keep their own calendar. A position carried over a weekend is unhedged for the
gap. The Nobitex rollover clock does not stop.

**Settlement lag.** If EcoGold settles T+1 while the short is instant, the legs
are not on at the same moment and the spread can move in between.

## 7. Still missing

Everything else is filled in. These two decide which structure wins:

- [ ] **MT5 swap long / swap short** on `GOLD_kilogram` and `GOLD_ounce`, and
      which weekday is charged 3×. Terminal → right-click symbol →
      *Specification*. Also worth grabbing while that dialog is open: contract
      size, tick value, min/max lot, lot step.
- [ ] **What it costs to move USD in and out of the MT5 broker account**, each
      direction — and whether capital can simply be parked there between trades.

Lower priority:

- [ ] Nobitex: confirm whether the pricing page's 0.03%/0.05% is **per 8-hour
      rollover** (→ 3× that per day, as assumed above) or already a daily
      figure. The help-page worked examples contradict each other on this.
- [ ] KuCoin LV0–VIP4 rates and `PAXG-USDT`'s fee class, from inside the account.
- [ ] Our current Nobitex volume tier — §5 assumes base (0.25%).

## 8. Code changes these imply

Not yet applied:

1. `exchangeFees[mt5]` → **0**. The spread is in the quote; 0.1% double-counts.
2. `exchangeFees[nobitex]` → **0.0025** for IRT markets (base tier), with a
   maker/taker and per-quote-currency split rather than one number per venue.
3. Move the whole table out of the `NewFinder` literal into config.
4. Add per-venue carry (`% per day`), margin (`mᵢ`) and profit-share tables,
   config-driven and defaulting to zero so nothing changes until they are set.
5. Compute `RT_req(D)` and `r_req` per pair for `D ∈ {1, 7}`; expose on
   `/arbitrage` and `/overview`.
6. Dashboard: show *"close when Return ≥ x%"* on a pair, and flag pairs whose
   recorded history (`/history`) shows the spread actually reverting that far
   within seven days. A threshold nothing ever reaches is not a trade.

Step 6 is what turns the existing history table from a chart into an entry
filter.
