import type { ExchangeBalances } from '../lib/api'
import { ageSeconds, formatAge, formatNumber } from '../lib/format'
import { EmptyState, Panel, SourceBadge } from './primitives'

export function Balances({ balances, now }: { balances: ExchangeBalances[]; now: number }) {
  const withValues = balances.filter((b) => Object.keys(b.balances).length > 0)

  return (
    <Panel
      title="Wallet balances"
      subtitle="Reported by each venue's authenticated API"
      right={withValues.length > 0 ? `${withValues.length} source` : null}
    >
      {withValues.length === 0 ? (
        <EmptyState
          title="No balances reported"
          hint="Balance fetchers only run when their API token is set and accepted. A rejected or expired token leaves this empty — check the backend logs for 401s."
        />
      ) : (
        <div className="divide-y divide-ink-800">
          {withValues.map((b) => {
            const entries = Object.entries(b.balances)
              .filter(([, v]) => v !== 0)
              .sort((a, c) => c[1] - a[1])
            return (
              <div key={b.exchange} className="px-4 py-3">
                <div className="mb-2 flex items-center justify-between gap-3">
                  <SourceBadge source={b.exchange} />
                  <span className="tnum text-xs text-ink-400">
                    {formatAge(ageSeconds(b.last_fetched, now))}
                  </span>
                </div>
                {entries.length === 0 ? (
                  <p className="text-xs text-ink-400">All wallets empty.</p>
                ) : (
                  <dl className="grid grid-cols-2 gap-x-4 gap-y-1.5 sm:grid-cols-3">
                    {entries.map(([currency, amount]) => (
                      <div key={currency} className="flex items-baseline justify-between gap-2">
                        <dt className="text-xs text-ink-400">{currency}</dt>
                        <dd className="tnum truncate text-sm text-ink-200">
                          {formatNumber(amount)}
                        </dd>
                      </div>
                    ))}
                  </dl>
                )}
              </div>
            )
          })}
        </div>
      )}
    </Panel>
  )
}
