import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  UnauthorizedError,
  apiBaseUrl,
  fetchOverview,
  getToken,
  setToken,
  type Overview,
} from './lib/api'
import { ageSeconds, formatAge, formatClock } from './lib/format'
import { ArbitrageChains } from './components/ArbitrageChains'
import { Backtest } from './components/Backtest'
import { ChainHistory } from './components/ChainHistory'
import { pairChains } from './lib/pairs'
import { Balances } from './components/Balances'
import { Orderbooks } from './components/Orderbooks'
import { TokenGate } from './components/TokenGate'
import { SourceBadge } from './components/primitives'

const POLL_INTERVAL_MS = 2000

type Connection = 'connecting' | 'live' | 'error'

function StatTile({
  label,
  value,
  tone = 'default',
  hint,
}: {
  label: string
  value: string
  tone?: 'default' | 'gain' | 'loss' | 'warn'
  hint?: string
}) {
  const toneClass = {
    default: 'text-ink-200',
    gain: 'text-gain-400',
    loss: 'text-loss-400',
    warn: 'text-warn-400',
  }[tone]

  return (
    <div className="rounded-xl border border-ink-700/70 bg-ink-900/60 px-4 py-3">
      <p className="text-[10px] uppercase tracking-wider text-ink-400">{label}</p>
      <p className={`tnum mt-1 text-xl font-semibold ${toneClass}`}>{value}</p>
      {hint && <p className="mt-0.5 truncate text-xs text-ink-400">{hint}</p>}
    </div>
  )
}

export default function App() {
  const [token, setTokenState] = useState(getToken())
  const [authFailed, setAuthFailed] = useState(false)
  const [data, setData] = useState<Overview | null>(null)
  const [connection, setConnection] = useState<Connection>('connecting')
  const [lastError, setLastError] = useState<string | null>(null)
  const [lastSuccessAt, setLastSuccessAt] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [paused, setPaused] = useState(false)

  const inFlight = useRef<AbortController | null>(null)

  const poll = useCallback(async () => {
    inFlight.current?.abort()
    const controller = new AbortController()
    inFlight.current = controller
    try {
      const overview = await fetchOverview(controller.signal)
      setData(overview)
      setConnection('live')
      setLastError(null)
      setLastSuccessAt(Date.now())
      setAuthFailed(false)
    } catch (err) {
      if (controller.signal.aborted) return
      if (err instanceof UnauthorizedError) {
        setAuthFailed(true)
        setConnection('error')
        return
      }
      setConnection('error')
      setLastError(err instanceof Error ? err.message : 'request failed')
    }
  }, [])

  // Live polling loop. Stops while the token gate is up so a rejected token
  // does not hammer the API with 401s every two seconds.
  useEffect(() => {
    if (paused || authFailed) return
    void poll()
    const id = setInterval(() => void poll(), POLL_INTERVAL_MS)
    return () => {
      clearInterval(id)
      inFlight.current?.abort()
    }
  }, [poll, paused, authFailed, token])

  // Local clock so "x seconds ago" keeps ticking between polls.
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [])

  const handleToken = (value: string) => {
    setToken(value)
    setTokenState(value)
    setAuthFailed(false)
    setConnection('connecting')
  }

  const chains = useMemo(() => data?.arbitrage ?? [], [data])
  const pairs = useMemo(() => pairChains(chains), [chains])
  const [historyKey, setHistoryKey] = useState<string | null>(null)
  const historyRef = useRef<HTMLDivElement>(null)
  const showHistory = useCallback((key: string) => {
    setHistoryKey(key)
    historyRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }, [])
  const books = useMemo(() => data?.orderbooks ?? [], [data])
  const sourceStats = data?.stats.source_stats ?? {}

  const bestChain = chains.length > 0 ? chains[0] : null
  const hasNobitex = (sourceStats['nobitex']?.orderbooks ?? 0) > 0

  const staleBooks = books.filter((b) => {
    const age = ageSeconds(b.updated_at, now)
    return age !== null && age > 60
  }).length

  if (authFailed && !data) {
    return (
      <TokenGate
        onSubmit={handleToken}
        error={token ? 'That token was rejected.' : undefined}
      />
    )
  }

  const statusTone =
    connection === 'live' ? 'text-gain-400' : connection === 'error' ? 'text-loss-400' : 'text-warn-400'
  const statusDot =
    connection === 'live'
      ? 'bg-gain-500 pulse-dot'
      : connection === 'error'
        ? 'bg-loss-500'
        : 'bg-warn-400 pulse-dot'

  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 lg:px-8">
      <header className="mb-6 flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-200">
            Arbi
            <span className="ml-2 text-sm font-normal text-ink-400">live arbitrage monitor</span>
          </h1>
          <p className="mt-0.5 truncate text-xs text-ink-500">{apiBaseUrl()}</p>
        </div>

        <div className="flex items-center gap-4">
          <span className={`inline-flex items-center gap-2 text-xs ${statusTone}`}>
            <span className={`h-2 w-2 rounded-full ${statusDot}`} />
            {connection === 'live' ? 'Live' : connection === 'error' ? 'Disconnected' : 'Connecting'}
            {lastSuccessAt && connection !== 'live' && (
              <span className="text-ink-400">
                · last {formatAge((now - lastSuccessAt) / 1000)}
              </span>
            )}
          </span>

          <button
            onClick={() => setPaused((v) => !v)}
            className="rounded-lg border border-ink-600 px-2.5 py-1 text-xs text-ink-300 transition hover:border-ink-400 hover:text-ink-200"
          >
            {paused ? 'Resume' : 'Pause'}
          </button>

          {token && (
            <button
              onClick={() => {
                setToken('')
                setTokenState('')
                setData(null)
                setAuthFailed(true)
              }}
              className="text-xs text-ink-400 transition hover:text-ink-200"
            >
              Lock
            </button>
          )}
        </div>
      </header>

      {connection === 'error' && lastError && (
        <div className="mb-4 rounded-lg border border-loss-500/30 bg-loss-500/8 px-4 py-2.5 text-xs text-loss-400">
          Cannot reach the API: {lastError}. Retrying every {POLL_INTERVAL_MS / 1000}s.
        </div>
      )}

      <div className="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5">
        <StatTile
          label="Best chain"
          value={bestChain ? `${bestChain.profit_percent > 0 ? '+' : ''}${bestChain.profit_percent.toFixed(3)}%` : '—'}
          tone={bestChain ? (bestChain.profit_percent > 0 ? 'gain' : 'loss') : 'default'}
          hint={bestChain?.path}
        />
        <StatTile
          label="Route pairs"
          value={String(pairs.length)}
          hint={`${chains.length} chains`}
        />
        <StatTile label="Markets" value={String(books.length)} />
        <StatTile
          label="Sources"
          value={String(data?.stats.registered_sources ?? 0)}
          hint={Object.entries(sourceStats)
            .map(([k, v]) => `${k} ${v.orderbooks}`)
            .join(' · ')}
        />
        <StatTile
          label="Stale markets"
          value={String(staleBooks)}
          tone={staleBooks > 0 ? 'warn' : 'default'}
          hint={`server ${formatClock(data?.server_time)}`}
        />
      </div>

      {data && Object.entries(sourceStats).some(([, v]) => v.orderbooks === 0) && (
        <div className="mb-4 flex flex-wrap items-center gap-2 rounded-lg border border-warn-400/25 bg-warn-400/8 px-4 py-2.5 text-xs text-warn-400">
          <span>No data from</span>
          {Object.entries(sourceStats)
            .filter(([, v]) => v.orderbooks === 0)
            .map(([name]) => (
              <SourceBadge key={name} source={name} />
            ))}
          <span className="text-ink-400">
            — the source is registered but has not delivered an order book.
          </span>
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <ArbitrageChains pairs={pairs} hasNobitex={hasNobitex} onShowHistory={showHistory} />
          <ChainHistory
            pairs={pairs}
            selected={historyKey}
            onSelect={setHistoryKey}
            panelRef={historyRef}
          />
          <Backtest />
          <Orderbooks books={books} now={now} />
        </div>
        <div className="space-y-4">
          <Balances balances={data?.balances ?? []} now={now} />
        </div>
      </div>

      <footer className="mt-8 text-center text-xs text-ink-500">
        Polling every {POLL_INTERVAL_MS / 1000}s{paused && ' · paused'}
      </footer>
    </div>
  )
}
