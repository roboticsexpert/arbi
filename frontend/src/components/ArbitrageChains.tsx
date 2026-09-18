import { useState } from 'react'
import type { ArbitrageChain, ChainStep } from '../lib/api'
import { formatCompact, formatNumber, formatPercent } from '../lib/format'
import { bestLeg, hop, roundTripPercent, type ChainPair } from '../lib/pairs'
import { EmptyState, Panel, SourceBadge } from './primitives'

function StepRow({ step }: { step: ChainStep }) {
  if (step.type === 'conversion' && step.fixed_conversion) {
    const c = step.fixed_conversion
    return (
      <tr className="border-t border-ink-800">
        <td className="py-1.5 pr-3">
          <SourceBadge source="conv" />
        </td>
        <td className="py-1.5 pr-3 text-ink-300">
          {c.from} → {c.to}
        </td>
        <td className="tnum py-1.5 pr-3 text-right text-ink-400">×{formatNumber(c.rate)}</td>
        <td className="tnum py-1.5 pr-3 text-right text-ink-300">{formatNumber(c.amount_in)}</td>
        <td className="tnum py-1.5 text-right text-ink-200">{formatNumber(c.amount_out)}</td>
      </tr>
    )
  }

  const t = step.trade
  if (!t) return null
  const isBuy = t.action.toLowerCase() === 'buy'
  return (
    <tr className="border-t border-ink-800">
      <td className="py-1.5 pr-3">
        <SourceBadge source={t.exchange} />
      </td>
      <td className="py-1.5 pr-3">
        <span className={isBuy ? 'text-gain-400' : 'text-loss-400'}>{isBuy ? 'BUY' : 'SELL'}</span>
        <span className="ml-1.5 text-ink-300">
          {t.base}/{t.quote}
        </span>
        {t.fee_percent > 0 && (
          <span className="ml-1.5 text-[10px] text-ink-400">fee {t.fee_percent}%</span>
        )}
      </td>
      <td className="tnum py-1.5 pr-3 text-right text-ink-400">{formatNumber(t.price)}</td>
      <td className="tnum py-1.5 pr-3 text-right text-ink-300">{formatNumber(t.amount)}</td>
      <td className="tnum py-1.5 text-right text-ink-200">{formatNumber(t.amount_out)}</td>
    </tr>
  )
}

function StepsTable({ chain }: { chain: ArbitrageChain }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[30rem] text-xs">
        <thead>
          <tr className="text-[10px] uppercase tracking-wider text-ink-400">
            <th className="pb-1 pr-3 text-left font-medium">Venue</th>
            <th className="pb-1 pr-3 text-left font-medium">Action</th>
            <th className="pb-1 pr-3 text-right font-medium">Price</th>
            <th className="pb-1 pr-3 text-right font-medium">Amount in</th>
            <th className="pb-1 text-right font-medium">Amount out</th>
          </tr>
        </thead>
        <tbody>
          {chain.steps.map((s, i) => (
            <StepRow key={i} step={s} />
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** IRT [ecogold] GOLD18 ⇢ GOLD24 [mt5] USD … — wraps instead of truncating. */
function Route({ chain }: { chain: ArbitrageChain }) {
  return (
    <span className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-sm text-ink-200" title={chain.path}>
      <span>{chain.start_currency}</span>
      {chain.steps.map((s, i) => {
        const h = hop(s)
        if (!h) return null
        return (
          <span key={i} className="inline-flex items-center gap-1.5">
            {h.venue === 'convert' ? (
              <span className="text-sky-300/80" title="fixed-rate conversion">
                ⇢
              </span>
            ) : (
              <SourceBadge source={h.venue} />
            )}
            <span>{h.to}</span>
          </span>
        )
      })}
    </span>
  )
}

const toneOf = (v: number | null) =>
  v === null ? 'text-ink-400' : v > 0 ? 'text-gain-400' : 'text-loss-400'

/** One direction of a pair: route on the left, result on the right. */
function LegRow({
  label,
  arrow,
  chain,
  best,
}: {
  label: string
  arrow: string
  chain: ArbitrageChain | null
  best: boolean
}) {
  return (
    <div className="flex items-center gap-3 py-1">
      <span className="w-16 shrink-0 text-[10px] font-semibold uppercase tracking-wider text-ink-400">
        <span className="mr-1 text-ink-300">{arrow}</span>
        {label}
      </span>
      {chain ? (
        <>
          <span className="min-w-0 flex-1">
            <Route chain={chain} />
            <span className="tnum mt-0.5 block text-[11px] text-ink-400">
              {formatCompact(chain.start_amount)} → {formatCompact(chain.end_amount)}{' '}
              {chain.end_currency}
              <span className="mx-1.5 text-ink-600">·</span>
              no fee {formatPercent(chain.profit_percent_no_fee)}
            </span>
          </span>
          <span className="shrink-0 text-right">
            <span className={`tnum block text-sm font-semibold ${toneOf(chain.profit_percent)}`}>
              {best && chain.profit_percent > 0 && (
                <span className="mr-1.5 rounded bg-gain-500/15 px-1 py-px text-[9px] font-semibold uppercase tracking-wider text-gain-400">
                  open
                </span>
              )}
              {formatPercent(chain.profit_percent)}
            </span>
            <span className={`tnum block text-[11px] opacity-80 ${toneOf(chain.profit_loss)}`}>
              {chain.profit_loss > 0 ? '+' : ''}
              {formatCompact(chain.profit_loss)} {chain.end_currency}
            </span>
          </span>
        </>
      ) : (
        <span className="flex-1 text-xs text-ink-400">
          Not available — one of its books has no liquidity on the needed side.
        </span>
      )}
    </div>
  )
}

function PairCard({
  pair,
  rank,
  onShowHistory,
}: {
  pair: ChainPair
  rank: number
  onShowHistory: (key: string) => void
}) {
  const [open, setOpen] = useState(rank === 0)
  const best = bestLeg(pair)
  const rt = roundTripPercent(pair)
  const rtNoFee = roundTripPercent(pair, true)
  const edge = best.profit_percent > 0 ? 'border-l-gain-500/60' : 'border-l-loss-500/50'

  return (
    <article className={`border-l-2 ${edge} bg-ink-850/40`}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-start gap-3 px-4 py-3 text-left transition hover:bg-ink-800/40"
        aria-expanded={open}
      >
        <span className="tnum mt-1.5 w-6 shrink-0 text-xs text-ink-400">#{rank + 1}</span>

        <span className="min-w-0 flex-1 divide-y divide-ink-800/70">
          <LegRow label="Go" arrow="→" chain={pair.go} best={best === pair.go} />
          <LegRow label="Return" arrow="←" chain={pair.ret} best={best === pair.ret} />
        </span>

        <span
          className="mt-1 w-24 shrink-0 border-l border-ink-800 pl-3 text-right"
          title="Open along Go and close along Return at today's prices. Roughly the spread paid for an immediate round trip."
        >
          <span className="block text-[10px] uppercase tracking-wider text-ink-400">Round trip</span>
          <span className={`tnum block text-base font-semibold ${toneOf(rt)}`}>
            {formatPercent(rt)}
          </span>
          <span className="tnum block text-[11px] text-ink-400">no fee {formatPercent(rtNoFee)}</span>
        </span>

        <svg
          className={`mt-2 h-4 w-4 shrink-0 text-ink-400 transition-transform ${open ? 'rotate-90' : ''}`}
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <path d="M6 4l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {open && (
        <div className="grid gap-4 px-4 pb-3 xl:grid-cols-2">
          <div className="xl:col-span-2">
            <button
              onClick={() => onShowHistory(pair.key)}
              className="inline-flex items-center gap-1.5 rounded-md border border-ink-700 px-2 py-1 text-[11px] font-medium text-ink-300 transition hover:border-ink-600 hover:text-ink-200"
            >
              <svg className="h-3.5 w-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden>
                <path d="M2 12l4-4 3 3 5-6" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              History chart
            </button>
          </div>
          {[
            { label: '→ Go', chain: pair.go },
            { label: '← Return', chain: pair.ret },
          ].map(
            ({ label, chain }) =>
              chain && (
                <div key={label} className="min-w-0">
                  <p className="mb-1 text-[10px] font-semibold uppercase tracking-wider text-ink-300">
                    {label}
                  </p>
                  <StepsTable chain={chain} />
                </div>
              ),
          )}
        </div>
      )}
    </article>
  )
}

export function ArbitrageChains({
  pairs,
  hasNobitex,
  onShowHistory,
}: {
  pairs: ChainPair[]
  hasNobitex: boolean
  onShowHistory: (key: string) => void
}) {
  const profitable = pairs.filter((p) => bestLeg(p).profit_percent > 0).length

  return (
    <Panel
      title="Arbitrage chains"
      subtitle="Each route paired with its reverse · recalculated by the backend every 10 seconds"
      right={
        pairs.length > 0 ? (
          <span>
            <span className={profitable > 0 ? 'text-gain-400' : 'text-ink-400'}>
              {profitable} profitable
            </span>
            <span className="mx-1.5 text-ink-600">/</span>
            {pairs.length} pairs
          </span>
        ) : null
      }
    >
      {pairs.length === 0 ? (
        <EmptyState
          title="No chains found"
          hint={
            hasNobitex
              ? 'The finder has not closed a loop with the currently connected sources.'
              : 'Nobitex is not connected, so there is no IRT ↔ crypto edge to close a loop with. Chains stay empty until an IRT market is available.'
          }
        />
      ) : (
        <div className="divide-y divide-ink-800">
          {pairs.map((p, i) => (
            <PairCard key={p.key} pair={p} rank={i} onShowHistory={onShowHistory} />
          ))}
        </div>
      )}
    </Panel>
  )
}
