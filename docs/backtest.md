# Backtest — replaying history as positions

Added 2026-09-18. Turns the recorded chain history into an answer to the only
question that matters before trading: **how many times could we have entered,
how many of those could we have closed, and what would each have made or lost?**

The cost model is the one derived in [trade-economics.md](trade-economics.md).
This document is about the replay: what it does, what it cannot see, and how to
read the result.

```
GET /backtest?from=…&to=…&min_entry=1&target=1&max_hold_days=7
```

Implementation: [`internal/backtest`](../backend/internal/backtest/backtest.go),
handler `getBacktest` in [`main.go`](../backend/main.go).

## The simulation

One position at a time, per Go/Return pair.

1. **Enter** at the first sample where the Go leg, after trading fees, is at or
   above `min_entry`.
2. **Hold**, walking the Return leg forward. At each sample compute days held
   `D`, the round trip `RT = (1+go)(1+ret) − 1`, and

   ```
   ROE(D) = K · [ RT − carry·D − transfer ] · (1 − share·(D−1))
   ```

   The profit-share haircut applies only when `ROE > 0`, matching how Nobitex
   charges it.
3. **Exit** at the first sample where `ROE ≥ target`.
4. If the target is never reached inside `max_hold_days`, **force-close** at the
   last sample in the window and book whatever that is worth — usually a loss.
5. Resume scanning after the exit. Entries during an open position are skipped:
   the equity is committed, so a second position would not have been fundable.

### Outcomes

| outcome | meaning | counted in P&L |
|---|---|---|
| `target` | reached the target and closed | yes |
| `timeout` | force-closed at `max_hold_days` | yes |
| `still_open` | the holding window runs past the end of the recorded data | **no** |
| `no_exit_data` | the Return leg has no history at all — one direction was never recorded | **no** |

`still_open` exists to stop the most recent `max_hold_days` of every run from
being charged with fake losses. A position opened two days before the data ends
has not timed out; we simply cannot know yet. Marking it as a timeout would
force-close it at a mid-move price and book the difference as a loss that never
happened. Both unresolved outcomes are reported but excluded from the totals,
the average and the win rate.

## Fees are applied at replay time, not read from history

The backtest reads **`avg_no_fee`**, not the stored `avg`.

The finder bakes its own `exchangeFees` table into `profit_percent` when the row
is written, so history recorded last week carries last week's assumptions and
cannot be re-costed. `avg_no_fee` is the raw spread, and every hop's venue is
recoverable from the path string
(`IRT-ecogold-GOLD18-convert-PAXG-kucoin-USDT-nobitex-IRT`), so the fee table is
applied here instead:

```
growth_with_fees = growth_no_fee × Π (1 − fee_venue)
```

which is exactly what the finder does per hop. Correcting a fee therefore
re-runs the backtest over existing data rather than invalidating it — which
matters, because two of the numbers in `finder.go` are wrong (see
[trade-economics.md §2](trade-economics.md), MT5 and Nobitex).

## Parameters

All percentages. Defaults are Structure A from trade-economics.md — EcoGold cash
long against a Nobitex 5x short, settling entirely inside Iran.

| query param | default | |
|---|---|---|
| `min_entry` | 1.0 | Go-leg profit, after fees, that opens a position |
| `target` | 1.0 | net return on equity that closes it |
| `max_hold_days` | 7 | force-close after this long |
| `capital_mult` | 0.83 | `K = 1/Σmᵢ` |
| `carry_per_day` | 0.15 | financing on notional, per day |
| `transfer` | 0.04 | one-off cost per round trip |
| `profit_share_per_day` | 0.5 | share of profit taken per extension day |
| `fee` | see below | `venue:fraction`, repeatable, e.g. `fee=nobitex:0.0025` |
| `path` | all | chain path, repeatable; the reverse is added automatically |
| `from` / `to` | last 30 days | unix seconds |
| `step` | auto | bucket minutes: 1, 5, 15, 60, 240, 1440 |
| `trades` | true | `false` returns summary counts only |

Default fees: `nobitex 0.0025`, `kucoin 0.001`, `binance 0.001`, `ecogold 0`,
`mt5 0`. MT5 is zero because the broker is spread-only and the spread is already
inside the quote the EA pushes — charging a percentage on top double-counts.

Sweeping a parameter is the point. The two unknowns in trade-economics.md §7
both enter here, so their effect can be measured rather than argued. The
dashboard exposes the sweep as three presets (see below); by curl:

```bash
curl "$API/backtest?carry_per_day=0&trades=false"                       # pool has spare capacity
curl "$API/backtest?capital_mult=0.99&carry_per_day=0.02&profit_share_per_day=0&trades=false"  # MT5 structure
```

## What it cannot see

Read these before trusting a result.

- **Bucket resolution.** Samples are bucket averages (`step` minutes, auto-chosen
  so a range stays under 3000 points; 30 days lands on 15 minutes). An exit can
  only be timed to the bucket, and an average hides the extremes *within* it.
  The reported `step` is in the response.
- **Averages, not touches.** `avg_no_fee` is the mean across the bucket, so a
  spike that existed for thirty seconds is invisible. This cuts both ways: it
  misses entries that were briefly available, and it refuses to fill you at a
  price that only printed once.
- **No depth.** The finder sizes chains against the book, but history stores only
  the percentage. Every fill is assumed to be for the full size at the recorded
  price. Thin books will not behave this way.
- **No slippage or latency** between the two legs. Both are assumed to execute
  at the recorded percentage, simultaneously.
- **Carry is linear.** `carry·D` with a constant daily rate. Nobitex actually
  charges per 8-hour rollover at a rate that floats with pool demand, and is
  free while the pool has capacity — so the real bill is lumpy and unknowable in
  advance. Run the sweep above to bound it.
- **No liquidation.** A position that would have been margin-called mid-hold is
  replayed as if it survived. At 5x this matters; a 20% adverse move on notional
  is the whole margin.
- **No FX hedge.** The unhedged rial exposure in trade-economics.md §6 is not
  modelled at all, in either direction.
- **Survivorship in the data.** A chain only has history for minutes in which
  both books had liquidity. Gaps are not interpolated, so a route that vanished
  during stress looks like it simply had no samples.

The honest summary: this bounds the *spread* opportunity. It does not prove
fills.

## In the dashboard

The **Backtest** panel sits under Chain history in
[`Backtest.tsx`](../frontend/src/components/Backtest.tsx). Controls are range
(7D/30D), hold limit (1/3/7 days), entry and target thresholds, and a structure
preset:

| preset | K | carry | profit share | |
|---|---|---|---|---|
| **Nobitex 5x** | 0.83 | 0.15%/day | 0.5%/day | Structure A — settles inside Iran |
| **MT5 1:100** | 0.99 | 0.02%/day | 0 | Structure B — swap is still a guess |
| **No carry** | 0.83 | 0 | 0.5%/day | Structure A on a day the lending pool has room |

"No carry" is not wishful thinking: Nobitex only charges rollover once its
lending pool is exhausted, so it is the same trade on a day the pool has spare
capacity. Toggling between it and **Nobitex 5x** is the fastest way to see how
much the unknown rollover rate actually decides.

Summaries load for every route (`trades=false`); a route's trade list is fetched
only while its row is expanded, and parameter edits are debounced, because the
endpoint reads the whole range at bucket resolution.

## Reading the result

```json
{
  "entries": 3, "exits": 1, "timeouts": 1,
  "still_open": 1, "no_exit_data": 0,
  "net_total_percent": 1.13, "net_avg_percent": 0.57, "win_rate_percent": 100,
  "step": 15,
  "pairs": [ { "path": "…", "trades": [ … ] } ]
}
```

`entries` is the opportunity count, `exits` is how many of them actually paid,
and `net_total_percent` sums return on equity across closed trades — it is *not*
an account return, because positions are sequential and the equity is redeployed
each time. `win_rate_percent` is over closed trades only.

Pairs are sorted by `net_total_percent`. Per-pair trade lists are capped at 200
entries; the counts are always complete.

## Still to do

- Wire `RequiredRoundTripPercent` into `/overview` so a live pair shows
  *"close when Return ≥ x%"* alongside the backtest evidence that the spread has
  historically reverted that far.
- Record order-book depth alongside the percentage, so fills can be sized rather
  than assumed. This is the biggest gap between the replay and reality.
- Let the fee table be edited from the panel; right now only `fee=venue:rate` on
  the URL can override it.
