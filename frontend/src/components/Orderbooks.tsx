import { useEffect, useRef, useState } from 'react'
import type { OrderBook } from '../lib/api'
import { ageSeconds, formatAge, formatPriceString } from '../lib/format'
import { EmptyState, Panel, SourceBadge } from './primitives'

/** Flags the direction a value moved since the previous poll, for the flash. */
function useDirection(value: string | undefined) {
  const prev = useRef<string | undefined>(undefined)
  const [dir, setDir] = useState<'up' | 'down' | null>(null)

  useEffect(() => {
    const before = prev.current
    prev.current = value
    if (before === undefined || value === undefined || before === value) return
    const a = Number(before)
    const b = Number(value)
    if (Number.isNaN(a) || Number.isNaN(b)) return
    setDir(b > a ? 'up' : 'down')
    const t = setTimeout(() => setDir(null), 900)
    return () => clearTimeout(t)
  }, [value])

  return dir
}

const STALE_AFTER_SECONDS = 60

function BookRow({ book, now }: { book: OrderBook; now: number }) {
  const bid = book.bids[0]?.price
  const ask = book.asks[0]?.price
  const bidDir = useDirection(bid)
  const askDir = useDirection(ask)

  const bidN = Number(bid)
  const askN = Number(ask)
  const spread =
    Number.isFinite(bidN) && Number.isFinite(askN) && bidN > 0 ? ((askN - bidN) / bidN) * 100 : null

  const age = ageSeconds(book.updated_at, now)
  const stale = age !== null && age > STALE_AFTER_SECONDS

  return (
    <tr className="border-t border-ink-800/80 transition hover:bg-ink-800/30">
      <td className="py-2 pl-4 pr-3">
        <SourceBadge source={book.source} />
      </td>
      <td className="py-2 pr-3 text-sm text-ink-200">
        {book.base}
        <span className="text-ink-500">/</span>
        {book.quote}
      </td>
      <td
        className={`tnum py-2 pr-3 text-right text-sm text-gain-400 ${
          bidDir === 'up' ? 'flash-up' : bidDir === 'down' ? 'flash-down' : ''
        }`}
      >
        {formatPriceString(bid)}
      </td>
      <td
        className={`tnum py-2 pr-3 text-right text-sm text-loss-400 ${
          askDir === 'up' ? 'flash-up' : askDir === 'down' ? 'flash-down' : ''
        }`}
      >
        {formatPriceString(ask)}
      </td>
      <td className="tnum py-2 pr-3 text-right text-xs text-ink-400">
        {spread === null ? '—' : `${spread.toFixed(3)}%`}
      </td>
      <td className="tnum py-2 pr-3 text-right text-xs text-ink-400">
        {book.bids.length}/{book.asks.length}
      </td>
      <td className="py-2 pr-4 text-right">
        <span
          className={`tnum inline-flex items-center gap-1.5 text-xs ${
            stale ? 'text-warn-400' : 'text-ink-400'
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              stale ? 'bg-warn-400' : 'bg-gain-500 pulse-dot'
            }`}
          />
          {formatAge(age)}
        </span>
      </td>
    </tr>
  )
}

export function Orderbooks({ books, now }: { books: OrderBook[]; now: number }) {
  return (
    <Panel
      title="Order books"
      subtitle="Best bid / ask per market, live from every connected source"
      right={`${books.length} market${books.length === 1 ? '' : 's'}`}
    >
      {books.length === 0 ? (
        <EmptyState title="No order books yet" hint="Waiting for the first update from a source." />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[42rem]">
            <thead>
              <tr className="text-[10px] uppercase tracking-wider text-ink-400">
                <th className="py-2 pl-4 pr-3 text-left font-medium">Source</th>
                <th className="py-2 pr-3 text-left font-medium">Market</th>
                <th className="py-2 pr-3 text-right font-medium">Best bid</th>
                <th className="py-2 pr-3 text-right font-medium">Best ask</th>
                <th className="py-2 pr-3 text-right font-medium">Spread</th>
                <th className="py-2 pr-3 text-right font-medium">Depth</th>
                <th className="py-2 pr-4 text-right font-medium">Updated</th>
              </tr>
            </thead>
            <tbody>
              {books.map((b) => (
                <BookRow key={`${b.source}:${b.base}-${b.quote}`} book={b} now={now} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Panel>
  )
}
