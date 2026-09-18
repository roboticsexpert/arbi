import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ColorType,
  CrosshairMode,
  LineSeries,
  LineStyle,
  TickMarkType,
  createChart,
  type IChartApi,
  type ISeriesApi,
  type LineData,
  type MouseEventParams,
  type Time,
  type UTCTimestamp,
  type WhitespaceData,
} from 'lightweight-charts'
import {
  HistoryDisabledError,
  UnauthorizedError,
  fetchHistory,
  fetchHistoryPaths,
  type HistoryPathInfo,
  type HistoryPoint,
  type HistoryResponse,
} from '../lib/api'
import { formatPercent } from '../lib/format'
import { compoundPercent, goAndReturn, type ChainPair } from '../lib/pairs'
import { EmptyState, Panel, SourceBadge } from './primitives'

const REFRESH_MS = 60_000

const RANGES = [
  { id: '1h', label: '1H', seconds: 3600 },
  { id: '6h', label: '6H', seconds: 6 * 3600 },
  { id: '24h', label: '24H', seconds: 24 * 3600 },
  { id: '7d', label: '7D', seconds: 7 * 86400 },
  { id: '30d', label: '30D', seconds: 30 * 86400 },
] as const
type RangeId = (typeof RANGES)[number]['id']

// Categorical slots 1 and 2 of the validated dark palette (blue / orange);
// round trip is a neutral dashed line so it never reads as a third route.
const COLORS = {
  go: '#3987e5',
  ret: '#d95926',
  rt: '#94a0b3', // ink-300
  text: '#94a0b3',
  grid: '#161c28', // ink-800
  border: '#1f2736', // ink-700
  zero: '#6b7688', // ink-400
}

interface Option {
  key: string // Go path
  go: string
  ret: string
  live: boolean
  lastSeen: number
}

/** Venues along a path, e.g. ecogold › mt5 › nobitex. */
function venuesOf(path: string): string[] {
  return path.split('-').filter((tok, i) => i % 2 === 1 && tok !== 'convert')
}

function buildOptions(pairs: ChainPair[], paths: HistoryPathInfo[]): Option[] {
  const out = new Map<string, Option>()
  pairs.forEach((p) => {
    const { go, ret } = goAndReturn(p.key)
    out.set(p.key, { key: p.key, go, ret, live: true, lastSeen: Infinity })
  })
  for (const info of paths) {
    const { go, ret } = goAndReturn(info.path)
    const existing = out.get(go)
    if (existing) existing.lastSeen = Math.max(existing.lastSeen, info.last_seen)
    else out.set(go, { key: go, go, ret, live: false, lastSeen: info.last_seen })
  }
  // live pairs keep the chain panel's order (Map insertion), then history-only by recency
  const all = [...out.values()]
  return [...all.filter((o) => o.live), ...all.filter((o) => !o.live).sort((a, b) => b.lastSeen - a.lastSeen)]
}

const localTime = (t: number, withDate: boolean) =>
  new Date(t * 1000).toLocaleString('en-GB', {
    ...(withDate ? { day: '2-digit', month: 'short' } : {}),
    hour: '2-digit',
    minute: '2-digit',
  })

type Series = (LineData | WhitespaceData)[]

/**
 * Points to chart data over a fixed bucket grid [from, to). Every bucket with
 * no point becomes a whitespace entry: the time scale is index-based, so
 * without them a two-hour outage (no liquidity, source down) would collapse
 * into one bar and look like a vertical jump instead of a gap.
 */
function toSeries<P extends { t: number }>(
  points: P[],
  grid: { from: number; to: number; step: number },
  value: (p: P) => number,
): Series {
  const byT = new Map(points.map((p) => [p.t, p]))
  const out: Series = []
  const start = Math.ceil(grid.from / grid.step) * grid.step
  let lastValue: LineData | null = null
  for (let t = start; t < grid.to; t += grid.step) {
    const p = byT.get(t)
    if (p) {
      lastValue = { time: t as UTCTimestamp, value: value(p) }
      out.push(lastValue)
      continue
    }
    // The library still joins points across whitespace. A segment takes the
    // colour of the point it starts from, so the last point before a gap is
    // made transparent to break the line.
    if (lastValue) lastValue.color = 'transparent'
    lastValue = null
    out.push({ time: t as UTCTimestamp })
  }
  return out
}

interface Hover {
  x: number
  t: number
  go?: HistoryPoint
  ret?: HistoryPoint
  rt?: number
}

function Swatch({ color, dashed }: { color: string; dashed?: boolean }) {
  return (
    <svg width="18" height="8" className="shrink-0" aria-hidden>
      <line
        x1="1"
        x2="17"
        y1="4"
        y2="4"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
        strokeDasharray={dashed ? '3 3' : undefined}
      />
    </svg>
  )
}

function RouteLine({ path }: { path: string }) {
  const tokens = path.split('-')
  return (
    <span className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-ink-300">
      <span>{tokens[0]}</span>
      {tokens.slice(1).map((tok, i) =>
        i % 2 === 0 ? (
          tok === 'convert' ? (
            <span key={i} className="text-sky-300/80">
              ⇢
            </span>
          ) : (
            <SourceBadge key={i} source={tok} />
          )
        ) : (
          <span key={i}>{tok}</span>
        ),
      )}
    </span>
  )
}

export function ChainHistory({
  pairs,
  selected,
  onSelect,
  panelRef,
}: {
  pairs: ChainPair[]
  selected: string | null
  onSelect: (key: string) => void
  panelRef?: React.Ref<HTMLDivElement>
}) {
  const [paths, setPaths] = useState<HistoryPathInfo[]>([])
  const [range, setRange] = useState<RangeId>('24h')
  const [noFee, setNoFee] = useState(false)
  const [data, setData] = useState<HistoryResponse | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [disabled, setDisabled] = useState(false)
  const [showTable, setShowTable] = useState(false)
  const [hover, setHover] = useState<Hover | null>(null)

  const options = useMemo(() => buildOptions(pairs, paths), [pairs, paths])
  const current = options.find((o) => o.key === selected) ?? options[0] ?? null
  // The chart container only exists once there is something to pick.
  const showChart = !disabled && options.length > 0

  // ---- data ----

  useEffect(() => {
    const ctl = new AbortController()
    const load = () =>
      fetchHistoryPaths(ctl.signal)
        .then((p) => {
          setPaths(p)
          setDisabled(false)
        })
        .catch((e) => {
          if (e instanceof HistoryDisabledError) setDisabled(true)
        })
    load()
    const id = setInterval(load, REFRESH_MS)
    return () => {
      ctl.abort()
      clearInterval(id)
    }
  }, [])

  const goPath = current?.go
  const retPath = current?.ret
  useEffect(() => {
    if (!goPath || !retPath) return
    const ctl = new AbortController()
    const seconds = RANGES.find((r) => r.id === range)!.seconds
    const load = () => {
      const to = Math.floor(Date.now() / 1000)
      fetchHistory([goPath, retPath], to - seconds, to, ctl.signal)
        .then((res) => {
          setData(res)
          setError(null)
        })
        .catch((e) => {
          if (ctl.signal.aborted) return
          if (e instanceof HistoryDisabledError) setDisabled(true)
          else if (e instanceof UnauthorizedError) setError('Unauthorized')
          else setError(e instanceof Error ? e.message : String(e))
        })
    }
    setData(null)
    load()
    const id = setInterval(load, REFRESH_MS)
    return () => {
      ctl.abort()
      clearInterval(id)
    }
  }, [goPath, retPath, range])

  const goPts = (goPath && data?.series[goPath]) || []
  const retPts = (retPath && data?.series[retPath]) || []
  const pick = (p: HistoryPoint) => (noFee ? p.avg_no_fee : p.avg)

  const rtPts = useMemo(() => {
    const byT = new Map(retPts.map((p) => [p.t, p]))
    return goPts.flatMap((g) => {
      const r = byT.get(g.t)
      return r ? [{ t: g.t, value: compoundPercent(pick(g), pick(r)) }] : []
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, noFee])

  // ---- chart ----

  const hostRef = useRef<HTMLDivElement>(null)
  const lastFit = useRef('')
  const chartRef = useRef<IChartApi | null>(null)
  const seriesRef = useRef<{ go: ISeriesApi<'Line'>; ret: ISeriesApi<'Line'>; rt: ISeriesApi<'Line'> } | null>(
    null,
  )
  const lookupRef = useRef({ go: new Map<number, HistoryPoint>(), ret: new Map<number, HistoryPoint>(), rt: new Map<number, number>() })

  useEffect(() => {
    const host = hostRef.current
    if (!showChart || !host) return
    const chart = createChart(host, {
      autoSize: true,
      layout: {
        background: { type: ColorType.Solid, color: 'transparent' },
        textColor: COLORS.text,
        fontSize: 11,
        fontFamily: 'inherit',
        attributionLogo: false,
      },
      grid: { vertLines: { visible: false }, horzLines: { color: COLORS.grid } },
      rightPriceScale: { borderColor: COLORS.border },
      timeScale: {
        borderColor: COLORS.border,
        timeVisible: true,
        secondsVisible: false,
        tickMarkFormatter: (time: Time, type: TickMarkType) => {
          const d = new Date((time as number) * 1000)
          if (type === TickMarkType.Year) return String(d.getFullYear())
          if (type === TickMarkType.Month) return d.toLocaleString('en-GB', { month: 'short' })
          if (type === TickMarkType.DayOfMonth) return d.toLocaleString('en-GB', { day: '2-digit', month: 'short' })
          return d.toLocaleString('en-GB', { hour: '2-digit', minute: '2-digit' })
        },
      },
      localization: {
        timeFormatter: (time: Time) => localTime(time as number, true),
        priceFormatter: (v: number) => `${v.toFixed(3)}%`,
      },
      crosshair: {
        mode: CrosshairMode.Magnet,
        vertLine: { color: COLORS.zero, labelVisible: false },
        horzLine: { color: COLORS.zero },
      },
    })

    const common = {
      lineWidth: 2 as const,
      priceLineVisible: false,
      lastValueVisible: true,
      crosshairMarkerRadius: 4,
      priceFormat: { type: 'custom' as const, formatter: (v: number) => `${v.toFixed(3)}%`, minMove: 0.001 },
    }
    const go = chart.addSeries(LineSeries, { ...common, color: COLORS.go, title: 'Go' })
    const ret = chart.addSeries(LineSeries, { ...common, color: COLORS.ret, title: 'Return' })
    const rt = chart.addSeries(LineSeries, {
      ...common,
      color: COLORS.rt,
      lineWidth: 1,
      lineStyle: LineStyle.Dashed,
      title: 'Round trip',
    })
    go.createPriceLine({ price: 0, color: COLORS.zero, lineWidth: 1, lineStyle: LineStyle.Dotted, axisLabelVisible: false, title: '' })

    const onMove = (param: MouseEventParams) => {
      if (param.time === undefined || !param.point) {
        setHover(null)
        return
      }
      const t = param.time as number
      const l = lookupRef.current
      setHover({ x: param.point.x, t, go: l.go.get(t), ret: l.ret.get(t), rt: l.rt.get(t) })
    }
    chart.subscribeCrosshairMove(onMove)

    chartRef.current = chart
    seriesRef.current = { go, ret, rt }
    return () => {
      chart.unsubscribeCrosshairMove(onMove)
      chart.remove()
      chartRef.current = null
      seriesRef.current = null
      lastFit.current = ''
    }
  }, [showChart])

  // Refit only when the question changes (pair or range), not on the 60s
  // refresh, so a zoom the user set survives the next poll.
  const fitKey = `${goPath}|${range}`

  useEffect(() => {
    const s = seriesRef.current
    if (!s) return
    if (!data) {
      // switching route: clear the previous one rather than show it under "Loading…"
      s.go.setData([])
      s.ret.setData([])
      s.rt.setData([])
      return
    }
    const grid = { from: data.from, to: data.to, step: data.step * 60 }
    s.go.setData(toSeries(goPts, grid, pick))
    s.ret.setData(toSeries(retPts, grid, pick))
    s.rt.setData(toSeries(rtPts, grid, (p) => p.value))
    lookupRef.current = {
      go: new Map(goPts.map((p) => [p.t, p])),
      ret: new Map(retPts.map((p) => [p.t, p])),
      rt: new Map(rtPts.map((p) => [p.t, p.value])),
    }
    if (lastFit.current !== fitKey) {
      chartRef.current?.timeScale().fitContent()
      lastFit.current = fitKey
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, noFee, rtPts, showChart])

  // ---- render ----

  const latest = {
    go: goPts.at(-1),
    ret: retPts.at(-1),
    rt: rtPts.at(-1)?.value,
  }
  const hasPoints = goPts.length > 0 || retPts.length > 0

  const legend = [
    { id: 'go', label: 'Go →', color: COLORS.go, path: goPath, value: latest.go ? pick(latest.go) : undefined },
    { id: 'ret', label: 'Return ←', color: COLORS.ret, path: retPath, value: latest.ret ? pick(latest.ret) : undefined },
    { id: 'rt', label: 'Round trip', color: COLORS.rt, dashed: true, value: latest.rt },
  ]

  const tableRows = useMemo(() => {
    const ts = [...new Set([...goPts, ...retPts].map((p) => p.t))].sort((a, b) => b - a).slice(0, 240)
    const g = new Map(goPts.map((p) => [p.t, p]))
    const r = new Map(retPts.map((p) => [p.t, p]))
    return ts.map((t) => {
      const gp = g.get(t)
      const rp = r.get(t)
      return { t, go: gp, ret: rp, rt: gp && rp ? compoundPercent(pick(gp), pick(rp)) : undefined }
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, noFee])

  return (
    <div ref={panelRef} className="scroll-mt-4">
      <Panel
        title="Chain history"
        subtitle={
          data
            ? `Go and Return of one route over time · ${data.step === 1 ? '1-minute' : `${data.step}-minute`} buckets · refreshes every minute`
            : 'Go and Return of one route over time · 1-minute buckets'
        }
      >
        {disabled ? (
          <EmptyState
            title="History is disabled on the backend"
            hint="The backend could not open its history database (HISTORY_DB_PATH). Check its logs for a [History] line."
          />
        ) : !showChart ? (
          <EmptyState title="No routes yet" hint="History starts filling as soon as the finder produces chains." />
        ) : (
          <div className="space-y-3 px-4 py-3">
            {/* filters: one row above the chart */}
            <div className="flex flex-wrap items-center gap-2">
              <select
                value={current?.key ?? ''}
                onChange={(e) => onSelect(e.target.value)}
                className="min-w-0 flex-1 rounded-md border border-ink-700 bg-ink-850 px-2 py-1.5 text-xs text-ink-200 outline-none focus:border-ink-600"
                aria-label="Route"
              >
                {options.map((o) => (
                  <option key={o.key} value={o.key}>
                    {o.live ? '● ' : '○ '}
                    {venuesOf(o.go).join(' › ')} — {o.go}
                  </option>
                ))}
              </select>

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

              <div className="flex rounded-md border border-ink-700 p-0.5" role="group" aria-label="Fees">
                {[
                  { v: false, label: 'With fee' },
                  { v: true, label: 'No fee' },
                ].map((f) => (
                  <button
                    key={f.label}
                    onClick={() => setNoFee(f.v)}
                    aria-pressed={noFee === f.v}
                    className={`rounded px-2 py-1 text-[11px] font-medium transition ${
                      noFee === f.v ? 'bg-ink-700 text-ink-200' : 'text-ink-400 hover:text-ink-200'
                    }`}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>

            {/* legend with latest values (direct labels) */}
            <div className="grid gap-2 sm:grid-cols-3">
              {legend.map((l) => (
                <div key={l.id} className="min-w-0 rounded-md border border-ink-800 bg-ink-850/40 px-2.5 py-2">
                  <div className="flex items-center justify-between gap-2">
                    <span className="flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wider text-ink-300">
                      <Swatch color={l.color} dashed={l.dashed} />
                      {l.label}
                    </span>
                    <span className="tnum text-sm font-semibold text-ink-200">{formatPercent(l.value)}</span>
                  </div>
                  {l.path ? (
                    <div className="mt-1">
                      <RouteLine path={l.path} />
                    </div>
                  ) : (
                    <p className="mt-1 text-[11px] text-ink-400">Open along Go, close along Return</p>
                  )}
                </div>
              ))}
            </div>

            <div className="relative h-80">
              <div ref={hostRef} className="absolute inset-0" />

              {hover && (hover.go || hover.ret) && (
                <div
                  className="pointer-events-none absolute top-2 z-10 w-56 rounded-md border border-ink-700 bg-ink-900/95 px-2.5 py-2 text-[11px] shadow-lg"
                  style={
                    hover.x > (hostRef.current?.clientWidth ?? 0) / 2
                      ? { left: Math.max(0, hover.x - 236) }
                      : { left: hover.x + 16 }
                  }
                >
                  <p className="mb-1 text-ink-400">{localTime(hover.t, true)}</p>
                  {[
                    { label: 'Go', color: COLORS.go, p: hover.go },
                    { label: 'Return', color: COLORS.ret, p: hover.ret },
                  ].map(({ label, color, p }) => (
                    <div key={label} className="flex items-center justify-between gap-2 py-0.5">
                      <span className="flex items-center gap-1.5 text-ink-300">
                        <Swatch color={color} />
                        {label}
                      </span>
                      <span className="tnum text-right text-ink-200">
                        {p ? (
                          <>
                            {formatPercent(pick(p))}
                            {!noFee && (
                              <span className="block text-[10px] text-ink-400">
                                {p.min.toFixed(3)} … {p.max.toFixed(3)}
                              </span>
                            )}
                          </>
                        ) : (
                          '—'
                        )}
                      </span>
                    </div>
                  ))}
                  <div className="mt-1 flex items-center justify-between gap-2 border-t border-ink-800 pt-1">
                    <span className="flex items-center gap-1.5 text-ink-300">
                      <Swatch color={COLORS.rt} dashed />
                      Round trip
                    </span>
                    <span className="tnum text-ink-200">{formatPercent(hover.rt)}</span>
                  </div>
                </div>
              )}

              {!hasPoints && (
                <div className="absolute inset-0 flex items-center justify-center">
                  <p className="max-w-sm text-center text-xs leading-relaxed text-ink-400">
                    {error
                      ? `Could not load history: ${error}`
                      : data
                        ? 'No history for this route in the selected range. Minutes are written once they complete.'
                        : 'Loading…'}
                  </p>
                </div>
              )}
            </div>

            <div className="flex items-center justify-between text-[11px] text-ink-400">
              <span>
                Lines are the average of each bucket; the tooltip shows its min … max (with fee). Round trip compounds
                Go and Return at the same minute.
              </span>
              {hasPoints && (
                <button onClick={() => setShowTable((v) => !v)} className="shrink-0 pl-3 text-ink-300 hover:text-ink-200">
                  {showTable ? 'Hide table' : 'Show table'}
                </button>
              )}
            </div>

            {showTable && hasPoints && (
              <div className="max-h-72 overflow-auto rounded-md border border-ink-800">
                <table className="w-full text-xs">
                  <thead className="sticky top-0 bg-ink-900">
                    <tr className="text-[10px] uppercase tracking-wider text-ink-400">
                      <th className="px-3 py-1.5 text-left font-medium">Time</th>
                      <th className="px-3 py-1.5 text-right font-medium">Go</th>
                      <th className="px-3 py-1.5 text-right font-medium">Return</th>
                      <th className="px-3 py-1.5 text-right font-medium">Round trip</th>
                    </tr>
                  </thead>
                  <tbody>
                    {tableRows.map((row) => (
                      <tr key={row.t} className="border-t border-ink-800">
                        <td className="tnum px-3 py-1 text-ink-400">{localTime(row.t, true)}</td>
                        <td className="tnum px-3 py-1 text-right text-ink-200">{row.go ? formatPercent(pick(row.go)) : '—'}</td>
                        <td className="tnum px-3 py-1 text-right text-ink-200">{row.ret ? formatPercent(pick(row.ret)) : '—'}</td>
                        <td className="tnum px-3 py-1 text-right text-ink-300">{formatPercent(row.rt)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </Panel>
    </div>
  )
}
