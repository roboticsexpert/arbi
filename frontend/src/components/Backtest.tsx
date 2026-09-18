// Backtest panel: replays recorded history as positions and shows how many
// entries there were, how many closed, and what each made or lost.
//
// The endpoint reads the whole range at bucket resolution, so this asks for
// summaries only (`trades=false`) and pulls a single route's trade list when a
// row is expanded. Parameter edits are debounced for the same reason.
//
// See docs/backtest.md for the cost model and what the replay cannot see.

import { useEffect, useMemo, useState } from 'react'
import {
  HistoryDisabledError,
  UnauthorizedError,
  fetchBacktest,
  type BacktestOutcome,
  type BacktestPair,
  type BacktestResult,
  type BacktestTrade,
} from '../lib/api'
import { EmptyState, Panel, SourceBadge } from './primitives'

const DEBOUNCE_MS = 400

const RANGES = [
  { id: '7d', label: '7D', seconds: 7 * 86400 },
  { id: '30d', label: '30D', seconds: 30 * 86400 },
] as const
type RangeId = (typeof RANGES)[number]['id']

const HOLDS = [1, 3, 7] as const

/**
 * The two executable structures from docs/trade-economics.md, plus the case
 * that isolates the single biggest unknown. Nobitex only charges rollover once
 * its lending pool is exhausted, so "No carry" is not a fantasy - it is the
 * same trade on a day when the pool has room.
 */
const STRUCTURES = [
  {
    id: 'nobitex',
    label: 'Nobitex 5x',
    hint: 'EcoGold cash long vs XAUT/USDT تعهدی. Settles inside Iran.',
    capitalMult: 0.83,
    carryPerDay: 0.15,
    profitSharePerDay: 0.5,
  },
  {
    id: 'mt5',
    label: 'MT5 1:100',
    hint: 'EcoGold cash long vs a gold CFD short. Swap is still a guess.',
    capitalMult: 0.99,
    carryPerDay: 0.02,
    profitSharePerDay: 0,
  },
  {
    id: 'nocarry',
    label: 'No carry',
    hint: 'Nobitex structure on a day the lending pool has spare capacity.',
    capitalMult: 0.83,
    carryPerDay: 0,
    profitSharePerDay: 0.5,
  },
] as const
type StructureId = (typeof STRUCTURES)[number]['id']

const OUTCOMES: Record<BacktestOutcome, { label: string; cls: string; title: string }> = {
  target: {
    label: 'target',
    cls: 'bg-gain-500/12 text-gain-400 ring-gain-500/25',
    title: 'Reached the target and closed',
  },
  timeout: {
    label: 'timeout',
    cls: 'bg-loss-500/12 text-loss-400 ring-loss-500/25',
    title: 'Force-closed at the holding limit',
  },
  still_open: {
    label: 'still open',
    cls: 'bg-warn-400/12 text-warn-400 ring-warn-400/25',
    title: 'The holding window runs past the end of the recorded data — excluded from the totals',
  },
  no_exit_data: {
    label: 'no exit data',
    cls: 'bg-ink-700/50 text-ink-300 ring-ink-600',
    title: 'The return leg was never recorded — excluded from the totals',
  },
}

/** Venues along a path, e.g. ecogold › mt5 › nobitex. */
const venuesOf = (path: string) => path.split('-').filter((tok, i) => i % 2 === 1 && tok !== 'convert')

const pct = (v: number, digits = 2) => `${v > 0 ? '+' : ''}${v.toFixed(digits)}%`
const toneOf = (v: number) => (v > 0 ? 'text-gain-400' : v < 0 ? 'text-loss-400' : 'text-ink-300')

const stamp = (t: number) =>
  new Date(t * 1000).toLocaleString('en-GB', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })

function Tile({ label, value, tone, hint }: { label: string; value: string; tone?: string; hint?: string }) {
  return (
    <div className="rounded-md border border-ink-800 bg-ink-850/40 px-2.5 py-2">
      <p className="text-[10px] uppercase tracking-wider text-ink-400">{label}</p>
      <p className={`tnum mt-0.5 text-base font-semibold ${tone ?? 'text-ink-200'}`}>{value}</p>
      {hint && <p className="mt-0.5 truncate text-[11px] text-ink-500">{hint}</p>}
    </div>
  )
}

function NumberField({
  label,
  value,
  onChange,
  step = 0.25,
  title,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  step?: number
  title?: string
}) {
  return (
    <label className="flex items-center gap-1.5 rounded-md border border-ink-700 px-2 py-1" title={title}>
      <span className="text-[11px] text-ink-400">{label}</span>
      <input
        type="number"
        value={value}
        step={step}
        onChange={(e) => {
          const n = Number(e.target.value)
          if (!Number.isNaN(n)) onChange(n)
        }}
        className="tnum w-14 bg-transparent text-right text-[11px] text-ink-200 outline-none"
      />
      <span className="text-[11px] text-ink-500">%</span>
    </label>
  )
}

function TradeRows({ trades }: { trades: BacktestTrade[] }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-[11px]">
        <thead className="text-ink-500">
          <tr className="text-left">
            <th className="py-1 pr-3 font-medium">Entered</th>
            <th className="py-1 pr-3 text-right font-medium">Hold</th>
            <th className="py-1 pr-3 text-right font-medium">Go</th>
            <th className="py-1 pr-3 text-right font-medium">Return</th>
            <th className="py-1 pr-3 text-right font-medium">Round trip</th>
            <th className="py-1 pr-3 text-right font-medium">Net</th>
            <th className="py-1 font-medium">Outcome</th>
          </tr>
        </thead>
        <tbody className="tnum">
          {trades.map((t) => {
            const o = OUTCOMES[t.outcome]
            const settled = t.outcome === 'target' || t.outcome === 'timeout'
            return (
              <tr key={`${t.entry_t}-${t.exit_t}`} className="border-t border-ink-800/70">
                <td className="py-1 pr-3 text-ink-300">{stamp(t.entry_t)}</td>
                <td className="py-1 pr-3 text-right text-ink-400">
                  {t.exit_t ? `${t.hold_days.toFixed(1)}d` : '—'}
                </td>
                <td className="py-1 pr-3 text-right text-ink-300">{pct(t.entry_percent)}</td>
                <td className="py-1 pr-3 text-right text-ink-300">
                  {t.exit_t ? pct(t.exit_percent) : '—'}
                </td>
                <td className="py-1 pr-3 text-right text-ink-300">
                  {t.exit_t ? pct(t.round_trip_percent) : '—'}
                </td>
                <td
                  className={`py-1 pr-3 text-right font-semibold ${settled ? toneOf(t.net_roe_percent) : 'text-ink-500'}`}
                >
                  {t.exit_t ? pct(t.net_roe_percent) : '—'}
                </td>
                <td className="py-1">
                  <span
                    title={o.title}
                    className={`inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset ${o.cls}`}
                  >
                    {o.label}
                  </span>
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
      <p className="mt-2 text-[11px] leading-relaxed text-ink-500">
        A <em>still open</em> position has not timed out — the data simply stops before its window does.
        Both it and <em>no exit data</em> are excluded from the totals so the most recent days are not
        charged with losses that never happened.
      </p>
    </div>
  )
}

function PairRow({
  pair,
  expanded,
  onToggle,
  trades,
  tradesLoading,
}: {
  pair: BacktestPair
  expanded: boolean
  onToggle: () => void
  trades: BacktestTrade[] | null
  tradesLoading: boolean
}) {
  const settled = pair.exits + pair.timeouts
  return (
    <>
      <tr
        onClick={onToggle}
        className="cursor-pointer border-t border-ink-800 transition hover:bg-ink-800/40"
      >
        <td className="py-2 pl-4 pr-3">
          <div className="flex items-center gap-1 whitespace-nowrap">
            {venuesOf(pair.path).map((v, i) => (
              <span key={`${v}-${i}`} className="flex items-center gap-1">
                {i > 0 && <span className="text-ink-600">›</span>}
                <SourceBadge source={v} />
              </span>
            ))}
          </div>
        </td>
        <td className="tnum py-2 pr-3 text-right text-ink-300">{pair.entries}</td>
        <td className="tnum py-2 pr-3 text-right text-gain-400">{pair.exits}</td>
        <td className="tnum py-2 pr-3 text-right text-loss-400">{pair.timeouts}</td>
        <td className="tnum py-2 pr-3 text-right text-ink-500">{pair.still_open}</td>
        <td className="tnum py-2 pr-3 text-right text-ink-400">
          {settled > 0 ? `${pair.avg_hold_days.toFixed(1)}d` : '—'}
        </td>
        <td className={`tnum py-2 pr-3 text-right font-semibold ${toneOf(pair.net_total_percent)}`}>
          {settled > 0 ? pct(pair.net_total_percent) : '—'}
        </td>
        <td className="tnum py-2 pr-4 text-right text-ink-300">
          {settled > 0 ? `${pair.win_rate_percent.toFixed(0)}%` : '—'}
        </td>
      </tr>
      {expanded && (
        <tr className="border-t border-ink-800 bg-ink-850/30">
          <td colSpan={8} className="px-4 py-3">
            <p className="mb-2 truncate font-mono text-[11px] text-ink-500">{pair.path}</p>
            {tradesLoading ? (
              <p className="text-xs text-ink-400">Loading trades…</p>
            ) : trades && trades.length > 0 ? (
              <TradeRows trades={trades} />
            ) : (
              <p className="text-xs text-ink-400">No trades on this route.</p>
            )}
          </td>
        </tr>
      )}
    </>
  )
}

export function Backtest() {
  const [range, setRange] = useState<RangeId>('30d')
  const [structure, setStructure] = useState<StructureId>('nobitex')
  const [minEntry, setMinEntry] = useState(1)
  const [target, setTarget] = useState(1)
  const [maxHoldDays, setMaxHoldDays] = useState<number>(7)

  const [result, setResult] = useState<BacktestResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [disabled, setDisabled] = useState(false)

  const [expanded, setExpanded] = useState<string | null>(null)
  const [trades, setTrades] = useState<BacktestTrade[] | null>(null)
  const [tradesLoading, setTradesLoading] = useState(false)

  const struct = STRUCTURES.find((s) => s.id === structure)!
  const seconds = RANGES.find((r) => r.id === range)!.seconds

  const query = useMemo(
    () => ({
      minEntry,
      target,
      maxHoldDays,
      capitalMult: struct.capitalMult,
      carryPerDay: struct.carryPerDay,
      profitSharePerDay: struct.profitSharePerDay,
    }),
    [minEntry, target, maxHoldDays, struct],
  )

  // Summaries for every route. Debounced because the numeric fields fire on
  // each keystroke and the endpoint reads the whole range.
  useEffect(() => {
    const ctl = new AbortController()
    setLoading(true)
    const id = setTimeout(() => {
      const to = Math.floor(Date.now() / 1000)
      fetchBacktest({ ...query, from: to - seconds, to, trades: false }, ctl.signal)
        .then((res) => {
          setResult(res)
          setError(null)
          setDisabled(false)
        })
        .catch((e) => {
          if (ctl.signal.aborted) return
          if (e instanceof HistoryDisabledError) setDisabled(true)
          else if (e instanceof UnauthorizedError) setError('Unauthorized')
          else setError(e instanceof Error ? e.message : String(e))
        })
        .finally(() => {
          if (!ctl.signal.aborted) setLoading(false)
        })
    }, DEBOUNCE_MS)
    return () => {
      ctl.abort()
      clearTimeout(id)
    }
  }, [query, seconds])

  // One route's trade list, fetched only while its row is open.
  useEffect(() => {
    if (!expanded) return
    const ctl = new AbortController()
    setTrades(null)
    setTradesLoading(true)
    const to = Math.floor(Date.now() / 1000)
    fetchBacktest({ ...query, from: to - seconds, to, path: expanded, trades: true }, ctl.signal)
      .then((res) => setTrades(res.pairs[0]?.trades ?? []))
      .catch(() => {
        if (!ctl.signal.aborted) setTrades([])
      })
      .finally(() => {
        if (!ctl.signal.aborted) setTradesLoading(false)
      })
    return () => ctl.abort()
  }, [expanded, query, seconds])

  const settled = result ? result.exits + result.timeouts : 0

  return (
    <Panel
      title="Backtest"
      subtitle="Recorded history replayed as positions"
      right={
        result ? (
          <span className="tnum">
            {result.step}m buckets{loading && ' · refreshing'}
          </span>
        ) : null
      }
    >
      {disabled ? (
        <EmptyState
          title="History is disabled on the backend"
          hint="The backend could not open its history database (HISTORY_DB_PATH). Check its logs for a [History] line."
        />
      ) : (
        <div className="space-y-3 px-4 py-3">
          <div className="flex flex-wrap items-center gap-2">
            <div className="flex rounded-md border border-ink-700 p-0.5" role="group" aria-label="Range">
              {RANGES.map((r) => (
                <button
                  key={r.id}
                  onClick={() => setRange(r.id)}
                  aria-pressed={range === r.id}
                  className={`rounded px-2 py-1 text-[11px] font-medium transition ${
                    range === r.id ? 'bg-ink-700 text-ink-200' : 'text-ink-400 hover:text-ink-200'
                  }`}
                >
                  {r.label}
                </button>
              ))}
            </div>

            <div className="flex rounded-md border border-ink-700 p-0.5" role="group" aria-label="Hold limit">
              {HOLDS.map((d) => (
                <button
                  key={d}
                  onClick={() => setMaxHoldDays(d)}
                  aria-pressed={maxHoldDays === d}
                  title={`Force-close after ${d} day${d > 1 ? 's' : ''}`}
                  className={`rounded px-2 py-1 text-[11px] font-medium transition ${
                    maxHoldDays === d ? 'bg-ink-700 text-ink-200' : 'text-ink-400 hover:text-ink-200'
                  }`}
                >
                  {d}D hold
                </button>
              ))}
            </div>

            <div className="flex rounded-md border border-ink-700 p-0.5" role="group" aria-label="Structure">
              {STRUCTURES.map((s) => (
                <button
                  key={s.id}
                  onClick={() => setStructure(s.id)}
                  aria-pressed={structure === s.id}
                  title={s.hint}
                  className={`rounded px-2 py-1 text-[11px] font-medium transition ${
                    structure === s.id ? 'bg-ink-700 text-ink-200' : 'text-ink-400 hover:text-ink-200'
                  }`}
                >
                  {s.label}
                </button>
              ))}
            </div>

            <NumberField
              label="Enter ≥"
              value={minEntry}
              onChange={setMinEntry}
              title="Go-leg profit, after fees, that opens a position"
            />
            <NumberField
              label="Target"
              value={target}
              onChange={setTarget}
              title="Net return on equity that closes a position"
            />
          </div>

          <p className="text-[11px] text-ink-500">
            {struct.hint} · K {struct.capitalMult} · carry {struct.carryPerDay}%/day · profit share{' '}
            {struct.profitSharePerDay}%/day
          </p>

          {error && (
            <div className="rounded-md border border-loss-500/30 bg-loss-500/8 px-3 py-2 text-xs text-loss-400">
              {error}
            </div>
          )}

          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6">
            <Tile label="Entries" value={result ? String(result.entries) : '—'} hint="could have opened" />
            <Tile
              label="Closed at target"
              value={result ? String(result.exits) : '—'}
              tone="text-gain-400"
            />
            <Tile label="Timed out" value={result ? String(result.timeouts) : '—'} tone="text-loss-400" />
            <Tile
              label="Still open"
              value={result ? String(result.still_open) : '—'}
              hint="data ends first"
            />
            <Tile
              label="Net total"
              value={result && settled > 0 ? pct(result.net_total_percent) : '—'}
              tone={result ? toneOf(result.net_total_percent) : undefined}
              hint={result && settled > 0 ? `avg ${pct(result.net_avg_percent)}` : undefined}
            />
            <Tile
              label="Win rate"
              value={result && settled > 0 ? `${result.win_rate_percent.toFixed(0)}%` : '—'}
              hint={`${settled} closed`}
            />
          </div>

          {!result && loading ? (
            <p className="py-6 text-center text-xs text-ink-400">Running…</p>
          ) : result && result.pairs.length === 0 ? (
            <EmptyState
              title="No routes with history"
              hint="History starts filling as soon as the finder produces chains, and a pair needs both directions recorded."
            />
          ) : result && result.entries === 0 ? (
            <EmptyState
              title="No entries in this range"
              hint={`No route's go leg reached ${minEntry}% after fees. Lower the entry threshold or widen the range.`}
            />
          ) : (
            result && (
              <div className="-mx-4 overflow-x-auto">
                <table className="w-full text-xs">
                  <thead className="text-ink-500">
                    <tr className="text-left">
                      <th className="py-1.5 pl-4 pr-3 font-medium">Route</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Entries</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Exits</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Timeouts</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Open</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Avg hold</th>
                      <th className="py-1.5 pr-3 text-right font-medium">Net</th>
                      <th className="py-1.5 pr-4 text-right font-medium">Win</th>
                    </tr>
                  </thead>
                  <tbody>
                    {result.pairs
                      .filter((p) => p.entries > 0)
                      .map((p) => (
                        <PairRow
                          key={p.path}
                          pair={p}
                          expanded={expanded === p.path}
                          onToggle={() => setExpanded((cur) => (cur === p.path ? null : p.path))}
                          trades={trades}
                          tradesLoading={tradesLoading}
                        />
                      ))}
                  </tbody>
                </table>
              </div>
            )
          )}

          <p className="text-[11px] leading-relaxed text-ink-500">
            Bounds the spread opportunity, not fills: samples are {result?.step ?? '—'}-minute bucket
            averages, and the replay assumes no depth limit, no slippage between the legs and no
            liquidation. See <span className="text-ink-400">docs/backtest.md</span>.
          </p>
        </div>
      )}
    </Panel>
  )
}
