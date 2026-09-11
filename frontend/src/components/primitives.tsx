import type { ReactNode } from 'react'

export function Panel({
  title,
  subtitle,
  right,
  children,
  className = '',
}: {
  title: string
  subtitle?: ReactNode
  right?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section
      className={`rounded-xl border border-ink-700/70 bg-ink-900/60 backdrop-blur-sm ${className}`}
    >
      <header className="flex items-baseline justify-between gap-4 border-b border-ink-700/60 px-4 py-3">
        <div className="min-w-0">
          <h2 className="truncate text-sm font-semibold tracking-wide text-ink-200">{title}</h2>
          {subtitle && <p className="mt-0.5 truncate text-xs text-ink-400">{subtitle}</p>}
        </div>
        {right && <div className="shrink-0 text-xs text-ink-400">{right}</div>}
      </header>
      {children}
    </section>
  )
}

export function SourceBadge({ source }: { source: string }) {
  const tone: Record<string, string> = {
    kucoin: 'bg-emerald-500/12 text-emerald-300 ring-emerald-500/25',
    nobitex: 'bg-amber-500/12 text-amber-300 ring-amber-500/25',
    ecogold: 'bg-yellow-500/12 text-yellow-200 ring-yellow-500/25',
    binance: 'bg-orange-500/12 text-orange-300 ring-orange-500/25',
    mt5: 'bg-violet-500/12 text-violet-300 ring-violet-500/25',
    conv: 'bg-sky-500/12 text-sky-300 ring-sky-500/25',
  }
  const cls = tone[source] ?? 'bg-ink-700/50 text-ink-300 ring-ink-600'
  return (
    <span
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider ring-1 ring-inset ${cls}`}
    >
      {source}
    </span>
  )
}

export function EmptyState({ title, hint }: { title: string; hint?: ReactNode }) {
  return (
    <div className="px-4 py-10 text-center">
      <p className="text-sm text-ink-300">{title}</p>
      {hint && <p className="mx-auto mt-1.5 max-w-md text-xs leading-relaxed text-ink-400">{hint}</p>}
    </div>
  )
}
