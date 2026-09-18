// Pairs every arbitrage chain with its reverse route.
//
// The finder's DFS emits both directions of a loop as separate chains, e.g.
//   IRT -ecogold-> GOLD18 -convert-> PAXG -kucoin-> USDT -nobitex-> IRT
//   IRT -nobitex-> USDT -kucoin-> PAXG -convert-> GOLD18 -ecogold-> IRT
// A position is opened along one and closed along the other, so the dashboard
// shows them together as a Go / Return pair.
//
// Matching works on the path string. Paths alternate asset and venue
// (`IRT-ecogold-GOLD18-convert-PAXG-…-IRT`), and asset symbols never contain a
// hyphen (pairs are configured as Base-Quote), so reversing the tokens yields
// exactly the reverse route's path. The same rule pairs live chains and paths
// that only exist in history.

import type { ArbitrageChain, ChainStep } from './api'

export interface ChainPair {
  /** The Go leg's path; stable across polls and shared with the history API. */
  key: string
  /** Either leg is missing when one of its books has no liquidity on the needed side. */
  go: ArbitrageChain | null
  ret: ArbitrageChain | null
}

export interface Hop {
  from: string
  venue: string
  to: string
}

export function hop(step: ChainStep): Hop | null {
  if (step.type === 'conversion' && step.fixed_conversion) {
    const c = step.fixed_conversion
    return { from: c.from, venue: 'convert', to: c.to }
  }
  const t = step.trade
  if (!t) return null
  // buy spends the quote to get the base; sell spends the base to get the quote
  return t.action.toLowerCase() === 'buy'
    ? { from: t.quote, venue: t.exchange, to: t.base }
    : { from: t.base, venue: t.exchange, to: t.quote }
}

export const reversePath = (path: string) => path.split('-').reverse().join('-')

/**
 * Direction is fixed by the route, not by which leg is better right now, so
 * the Go/Return labels don't swap between polls as prices move.
 */
export function goAndReturn(path: string): { go: string; ret: string } {
  const rev = reversePath(path)
  return path <= rev ? { go: path, ret: rev } : { go: rev, ret: path }
}

/** Ratio of end to start amount, i.e. 1.002 for +0.2%. */
const growth = (c: ArbitrageChain, noFee = false) =>
  (noFee ? c.end_amount_no_fee : c.end_amount) / c.start_amount

/**
 * Result of opening along Go and closing along Return right now, in percent.
 * Both legs start from the same IRT amount, so this compounds the two ratios;
 * it is effectively the spread you pay for an immediate round trip.
 */
export function roundTripPercent(pair: ChainPair, noFee = false): number | null {
  if (!pair.go || !pair.ret) return null
  return (growth(pair.go, noFee) * growth(pair.ret, noFee) - 1) * 100
}

/** Same as roundTripPercent, from two profit percentages. */
export const compoundPercent = (a: number, b: number) => ((1 + a / 100) * (1 + b / 100) - 1) * 100

export function bestLeg(pair: ChainPair): ArbitrageChain {
  if (!pair.go) return pair.ret!
  if (!pair.ret) return pair.go
  return pair.ret.profit_percent > pair.go.profit_percent ? pair.ret : pair.go
}

export function pairChains(chains: ArbitrageChain[]): ChainPair[] {
  const byPath = new Map(chains.map((c) => [c.path, c]))
  const pairs = new Map<string, ChainPair>()

  for (const c of chains) {
    const { go, ret } = goAndReturn(c.path)
    if (pairs.has(go)) continue
    pairs.set(go, {
      key: go,
      go: byPath.get(go) ?? null,
      ret: go === ret ? null : byPath.get(ret) ?? null,
    })
  }

  return [...pairs.values()].sort((a, b) => bestLeg(b).profit_percent - bestLeg(a).profit_percent)
}
