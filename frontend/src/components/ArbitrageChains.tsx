import { useState } from 'react'
import type { ArbitrageChain, ChainStep } from '../lib/api'
import { formatCompact, formatNumber, formatPercent } from '../lib/format'
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

function ChainCard({ chain, rank }: { chain: ArbitrageChain; rank: number }) {
  const [open, setOpen] = useState(rank === 0)
  const profitable = chain.profit_percent > 0
  const tone = profitable ? 'text-gain-400' : 'text-loss-400'
  const edge = profitable ? 'border-l-gain-500/60' : 'border-l-loss-500/50'

  return (
    <article className={`border-l-2 ${edge} bg-ink-850/40`}>
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition hover:bg-ink-800/40"
        aria-expanded={open}
      >
        <span className="tnum w-6 shrink-0 text-xs text-ink-400">#{rank + 1}</span>

        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm text-ink-200">{chain.path}</span>
          <span className="tnum mt-0.5 block text-xs text-ink-400">
            {formatCompact(chain.start_amount)} {chain.start_currency} →{' '}
            {formatCompact(chain.end_amount)} {chain.end_currency}
            <span className="mx-1.5 text-ink-600">·</span>
            no fee {formatPercent(chain.profit_percent_no_fee)}
          </span>
        </span>

        <span className="shrink-0 text-right">
          <span className={`tnum block text-base font-semibold ${tone}`}>
            {formatPercent(chain.profit_percent)}
          </span>
          <span className={`tnum block text-xs ${tone} opacity-80`}>
            {chain.profit_loss > 0 ? '+' : ''}
            {formatCompact(chain.profit_loss)} {chain.end_currency}
          </span>
        </span>

        <svg
          className={`h-4 w-4 shrink-0 text-ink-400 transition-transform ${open ? 'rotate-90' : ''}`}
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
        >
          <path d="M6 4l4 4-4 4" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {open && (
        <div className="overflow-x-auto px-4 pb-3">
          <table className="w-full min-w-[34rem] text-xs">
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
      )}
    </article>
  )
}

export function ArbitrageChains({
  chains,
  hasNobitex,
}: {
  chains: ArbitrageChain[]
  hasNobitex: boolean
}) {
  const profitable = chains.filter((c) => c.profit_percent > 0).length

  return (
    <Panel
      title="Arbitrage chains"
      subtitle="Recalculated by the backend every 10 seconds"
      right={
        chains.length > 0 ? (
          <span>
            <span className={profitable > 0 ? 'text-gain-400' : 'text-ink-400'}>
              {profitable} profitable
            </span>
            <span className="mx-1.5 text-ink-600">/</span>
            {chains.length} found
          </span>
        ) : null
      }
    >
      {chains.length === 0 ? (
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
          {chains.map((c, i) => (
            <ChainCard key={c.path + i} chain={c} rank={i} />
          ))}
        </div>
      )}
    </Panel>
  )
}
