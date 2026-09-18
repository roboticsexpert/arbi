package arbitrage

import "strings"

// Chain path helpers.
//
// buildChainFromPath writes paths as alternating asset and venue tokens:
//
//	IRT-ecogold-GOLD18-convert-PAXG-kucoin-USDT-nobitex-IRT
//
// Asset symbols never contain "-" (pairs are configured as Base-Quote), so
// reversing the tokens yields exactly the reverse route's path. The dashboard
// relies on the same rule in frontend/src/lib/pairs.ts - keep the two in step.

// ConvertToken marks a fixed-rate conversion rather than a venue.
const ConvertToken = "convert"

// ReversePath returns the path of the route walked in the opposite direction.
func ReversePath(path string) string {
	parts := strings.Split(path, "-")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "-")
}

// GoAndReturn names the two directions of a loop. Which one is "go" is fixed by
// the route (the lexicographically smaller path), not by which is better right
// now, so labels stay stable as prices move.
func GoAndReturn(path string) (goPath, retPath string) {
	rev := ReversePath(path)
	if path <= rev {
		return path, rev
	}
	return rev, path
}

// PathVenues lists the venue token of every trade hop in a path, in order.
// Conversion hops carry no venue and are skipped, so the result is exactly the
// hops that pay a trading fee.
func PathVenues(path string) []string {
	parts := strings.Split(path, "-")
	var venues []string
	// Tokens alternate asset, venue, asset, venue, ... so venues sit at odd
	// indices. A well-formed path has an odd number of tokens.
	for i := 1; i < len(parts); i += 2 {
		if parts[i] == ConvertToken {
			continue
		}
		venues = append(venues, parts[i])
	}
	return venues
}
