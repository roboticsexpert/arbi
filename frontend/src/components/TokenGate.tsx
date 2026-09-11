import { useState } from 'react'

export function TokenGate({
  onSubmit,
  error,
}: {
  onSubmit: (token: string) => void
  error?: string
}) {
  const [value, setValue] = useState('')

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          if (value.trim()) onSubmit(value.trim())
        }}
        className="w-full max-w-sm rounded-xl border border-ink-700/70 bg-ink-900/70 p-6 backdrop-blur"
      >
        <h1 className="text-lg font-semibold text-ink-200">Arbi</h1>
        <p className="mt-1 text-sm text-ink-400">
          This dashboard is protected. Enter the access token to continue.
        </p>

        <input
          type="password"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          autoFocus
          placeholder="Access token"
          className="mt-4 w-full rounded-lg border border-ink-600 bg-ink-950/60 px-3 py-2 text-sm text-ink-200 outline-none placeholder:text-ink-500 focus:border-accent-400/70"
        />

        {error && <p className="mt-2 text-xs text-loss-400">{error}</p>}

        <button
          type="submit"
          className="mt-4 w-full rounded-lg bg-accent-400/90 px-3 py-2 text-sm font-medium text-ink-950 transition hover:bg-accent-400 disabled:opacity-40"
          disabled={!value.trim()}
        >
          Unlock
        </button>
      </form>
    </div>
  )
}
