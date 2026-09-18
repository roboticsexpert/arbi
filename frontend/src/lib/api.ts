// API client for the Arbi backend.
//
// The API base URL is resolved at runtime from /config.js (written by the
// container entrypoint) so the same image can point at any backend, falling
// back to the build-time value and finally to the current origin.

declare global {
  interface Window {
    __ARBI_CONFIG__?: { apiUrl?: string }
  }
}

const TOKEN_KEY = 'arbi.dashboard.token'

export function apiBaseUrl(): string {
  const runtime = window.__ARBI_CONFIG__?.apiUrl
  if (runtime && !runtime.startsWith('__')) return runtime.replace(/\/$/, '')
  const buildTime = import.meta.env.VITE_API_URL as string | undefined
  if (buildTime) return buildTime.replace(/\/$/, '')
  return window.location.origin
}

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    return ''
  }
}

export function setToken(token: string) {
  try {
    if (token) localStorage.setItem(TOKEN_KEY, token)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* private mode - the token just won't persist across reloads */
  }
}

export class UnauthorizedError extends Error {
  constructor() {
    super('unauthorized')
  }
}

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${apiBaseUrl()}${path}`, {
    headers: { 'X-Dashboard-Token': getToken() },
    signal,
  })
  if (res.status === 401) throw new UnauthorizedError()
  if (!res.ok) throw new Error(`${path} failed: ${res.status}`)
  return (await res.json()) as T
}

// ---- Types mirroring the Go API ----

export interface PriceLevel {
  price: string
  quantity: string
}

export interface OrderBook {
  source: string
  base: string
  quote: string
  bids: PriceLevel[]
  asks: PriceLevel[]
  timestamp: number
  updated_at: string
}

export interface TradeStep {
  exchange: string
  action: string
  base: string
  quote: string
  price: number
  amount: number
  volume: number
  fee_percent: number
  amount_out: number
}

export interface FixedConversionStep {
  from: string
  to: string
  rate: number
  amount_in: number
  amount_out: number
}

export interface ChainStep {
  type: 'trade' | 'conversion'
  trade?: TradeStep
  fixed_conversion?: FixedConversionStep
}

export interface ArbitrageChain {
  steps: ChainStep[]
  start_currency: string
  end_currency: string
  start_amount: number
  end_amount: number
  profit_loss: number
  profit_percent: number
  end_amount_no_fee: number
  profit_loss_no_fee: number
  profit_percent_no_fee: number
  path: string
}

export interface ExchangeBalances {
  exchange: string
  balances: Record<string, number>
  last_fetched: string
}

export interface Stats {
  total_orderbooks: number
  registered_sources: number
  source_stats: Record<string, { orderbooks: number }>
}

export interface Overview {
  server_time: string
  stats: Stats
  orderbooks: OrderBook[]
  arbitrage: ArbitrageChain[] | null
  balances: ExchangeBalances[]
}

export function fetchOverview(signal?: AbortSignal) {
  return get<Overview>('/overview', signal)
}

// ---- Chain history ----

export class HistoryDisabledError extends Error {
  constructor() {
    super('history is disabled on the backend')
  }
}

export interface HistoryPathInfo {
  path: string
  first_seen: number
  last_seen: number
}

export interface HistoryPoint {
  t: number // unix seconds, bucket start
  avg: number
  min: number
  max: number
  last: number
  avg_no_fee: number
  samples: number
}

export interface HistoryResponse {
  from: number
  to: number
  step: number // minutes
  series: Record<string, HistoryPoint[]>
}

async function getHistory<T>(path: string, signal?: AbortSignal): Promise<T> {
  try {
    return await get<T>(path, signal)
  } catch (e) {
    if (e instanceof Error && e.message.endsWith(': 503')) throw new HistoryDisabledError()
    throw e
  }
}

export function fetchHistoryPaths(signal?: AbortSignal) {
  return getHistory<HistoryPathInfo[]>('/history/paths', signal)
}

export function fetchHistory(paths: string[], from: number, to: number, signal?: AbortSignal) {
  const q = new URLSearchParams({ from: String(from), to: String(to) })
  for (const p of paths) q.append('path', p)
  return getHistory<HistoryResponse>(`/history?${q}`, signal)
}

export async function checkHealth(): Promise<boolean> {
  try {
    const res = await fetch(`${apiBaseUrl()}/up`)
    return res.ok
  } catch {
    return false
  }
}

// ---- Backtest ----
//
// Replays recorded history as positions. See docs/backtest.md. The request is
// expensive (it reads the whole range at bucket resolution), so the dashboard
// asks for summaries only and fetches a single route's trade list on demand.

export interface BacktestParams {
  target_percent: number
  min_entry_percent: number
  max_hold_days: number
  capital_mult: number
  carry_per_day_percent: number
  transfer_percent: number
  profit_share_per_day_percent: number
  fees: Record<string, number>
}

export type BacktestOutcome = 'target' | 'timeout' | 'still_open' | 'no_exit_data'

export interface BacktestTrade {
  entry_t: number
  exit_t: number
  hold_days: number
  entry_percent: number
  exit_percent: number
  round_trip_percent: number
  net_roe_percent: number
  outcome: BacktestOutcome
}

export interface BacktestPair {
  path: string
  reverse_path: string
  entries: number
  exits: number
  timeouts: number
  no_exit_data: number
  still_open: number
  net_total_percent: number
  net_avg_percent: number
  win_rate_percent: number
  avg_hold_days: number
  best_percent: number
  worst_percent: number
  trades: BacktestTrade[] | null
}

export interface BacktestResult {
  params: BacktestParams
  from: number
  to: number
  step: number // minutes
  pairs: BacktestPair[]
  entries: number
  exits: number
  timeouts: number
  no_exit_data: number
  still_open: number
  net_total_percent: number
  net_avg_percent: number
  win_rate_percent: number
}

export interface BacktestQuery {
  from: number
  to: number
  minEntry: number
  target: number
  maxHoldDays: number
  capitalMult: number
  carryPerDay: number
  profitSharePerDay: number
  /** Omit for every route; set to pull one route's trade list. */
  path?: string
  trades?: boolean
}

export function fetchBacktest(q: BacktestQuery, signal?: AbortSignal) {
  const params = new URLSearchParams({
    from: String(q.from),
    to: String(q.to),
    min_entry: String(q.minEntry),
    target: String(q.target),
    max_hold_days: String(q.maxHoldDays),
    capital_mult: String(q.capitalMult),
    carry_per_day: String(q.carryPerDay),
    profit_share_per_day: String(q.profitSharePerDay),
    trades: String(q.trades ?? false),
  })
  if (q.path) params.append('path', q.path)
  return getHistory<BacktestResult>(`/backtest?${params}`, signal)
}
