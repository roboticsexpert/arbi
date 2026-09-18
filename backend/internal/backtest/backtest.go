// Package backtest replays recorded chain history as if positions had been
// opened and closed, and reports how many entries there were, how many of them
// could actually be closed, and what each would have earned or lost.
//
// It works on the *no-fee* series, not the stored with-fee one. The finder bakes
// its own fee assumptions into profit_percent at record time, so history written
// last week carries last week's assumptions and cannot be re-costed. avg_no_fee
// is the raw spread, and the venue of every hop is recoverable from the path
// string, so fees are applied here instead - changing the fee table re-runs the
// backtest over existing data rather than invalidating it.
//
// The cost model is the one in docs/trade-economics.md:
//
//	RT     = (1+go)(1+ret) - 1
//	ROE(D) = K · [ RT - carry·D - transfer ] · (1 - share·(D-1))
//
// with the profit-share haircut applied only to a profitable position, matching
// how Nobitex charges it.
package backtest

import (
	"math"
	"sort"

	"arbi/internal/arbitrage"
)

// Sample is one bucket of a chain's history: the profit percentage the chain
// would have yielded with **no** fees applied.
type Sample struct {
	T            int64   // unix seconds, bucket start
	NoFeePercent float64 // profit %, fees excluded
}

// Params configures one run. Every field except Fees is a percentage, so 1.0
// means 1%. Fees are fractions, matching the finder's exchangeFees table.
type Params struct {
	// TargetPercent is the net return on equity a position must reach before it
	// is closed.
	TargetPercent float64 `json:"target_percent"`
	// MinEntryPercent is the go-leg profit (after fees) that opens a position.
	MinEntryPercent float64 `json:"min_entry_percent"`
	// MaxHoldDays is how long a position may stay open before it is force-closed
	// at whatever the return leg is worth.
	MaxHoldDays float64 `json:"max_hold_days"`

	// CapitalMult is K = 1/Σmᵢ, the notional-to-equity ratio.
	CapitalMult float64 `json:"capital_mult"`
	// CarryPerDayPercent is Σcᵢ: financing on the whole notional, per day.
	CarryPerDayPercent float64 `json:"carry_per_day_percent"`
	// TransferPercent is the one-off cost of moving capital, per round trip.
	TransferPercent float64 `json:"transfer_percent"`
	// ProfitSharePerDayPercent is taken out of a *profitable* position's return
	// for every day past the first (Nobitex charges 0.5%).
	ProfitSharePerDayPercent float64 `json:"profit_share_per_day_percent"`

	// Fees maps a venue token to its per-hop trading fee as a fraction.
	Fees map[string]float64 `json:"fees"`
}

// DefaultParams mirrors Structure A in docs/trade-economics.md: EcoGold cash
// long against a Nobitex 5x short, everything settling inside Iran.
func DefaultParams() Params {
	return Params{
		TargetPercent:            1.0,
		MinEntryPercent:          1.0,
		MaxHoldDays:              7,
		CapitalMult:              0.83,
		CarryPerDayPercent:       0.15,
		TransferPercent:          0.04,
		ProfitSharePerDayPercent: 0.5,
		Fees: map[string]float64{
			"nobitex": 0.0025, // IRT taker, base tier
			"kucoin":  0.001,
			"binance": 0.001,
			"ecogold": 0,
			"mt5":     0, // spread-only broker; the spread is already in the quote
		},
	}
}

// feeMultiplier is the fraction of an amount that survives every trading fee
// along a path. The finder applies fees multiplicatively per hop, so chain
// growth with fees is growth-without-fees times this.
func (p Params) feeMultiplier(path string) float64 {
	mul := 1.0
	for _, venue := range arbitrage.PathVenues(path) {
		fee, ok := p.Fees[venue]
		if !ok {
			fee = 0.001 // finder's default for an unknown venue
		}
		mul *= 1 - fee
	}
	return mul
}

// withFees converts a stored no-fee percentage into the percentage actually
// realised after every hop's trading fee.
func withFees(noFeePercent, feeMul float64) float64 {
	return ((1+noFeePercent/100)*feeMul - 1) * 100
}

// roePercent is the net return on equity of closing a position whose two legs
// compound to roundTrip (a fraction) after days held.
func (p Params) roePercent(roundTrip, days float64) float64 {
	net := roundTrip - p.CarryPerDayPercent/100*days - p.TransferPercent/100
	roe := p.CapitalMult * net
	if roe > 0 && p.ProfitSharePerDayPercent > 0 && days > 1 {
		share := math.Min(1, p.ProfitSharePerDayPercent/100*(days-1))
		roe *= 1 - share
	}
	return roe * 100
}

// RequiredRoundTripPercent is the round trip a position must reach after days
// held to clear the target. It is the formula in §5 of trade-economics.md and
// is what the dashboard should show as "close when the return leg reaches …".
func (p Params) RequiredRoundTripPercent(days float64) float64 {
	denom := p.CapitalMult
	if p.ProfitSharePerDayPercent > 0 && days > 1 {
		share := math.Min(0.99, p.ProfitSharePerDayPercent/100*(days-1))
		denom *= 1 - share
	}
	if denom <= 0 {
		return math.Inf(1)
	}
	return p.TargetPercent/denom + p.CarryPerDayPercent*days + p.TransferPercent
}

// Outcome of a single simulated position.
const (
	OutcomeTarget     = "target"       // closed at or above the target
	OutcomeTimeout    = "timeout"      // force-closed at MaxHoldDays
	OutcomeNoExitData = "no_exit_data" // the return leg had no history in the window
	// OutcomeOpen marks a position whose holding window runs past the end of the
	// recorded data. It has not timed out - we simply cannot know yet. Booking it
	// as a timeout would charge the most recent MaxHoldDays of every run with
	// losses that are an artefact of where the data stops.
	OutcomeOpen = "still_open"
)

// Trade is one simulated round trip.
type Trade struct {
	EntryT           int64   `json:"entry_t"`
	ExitT            int64   `json:"exit_t"`
	HoldDays         float64 `json:"hold_days"`
	EntryPercent     float64 `json:"entry_percent"`      // go leg, after fees
	ExitPercent      float64 `json:"exit_percent"`       // return leg, after fees
	RoundTripPercent float64 `json:"round_trip_percent"` // the two legs compounded
	NetROEPercent    float64 `json:"net_roe_percent"`    // after carry, transfer, profit share
	Outcome          string  `json:"outcome"`
}

// PairResult summarises every position simulated on one Go/Return pair.
type PairResult struct {
	Path        string `json:"path"`
	ReversePath string `json:"reverse_path"`

	Entries    int `json:"entries"`  // signals acted on
	Exits      int `json:"exits"`    // reached the target
	Timeouts   int `json:"timeouts"` // force-closed at MaxHoldDays
	NoExitData int `json:"no_exit_data"`
	StillOpen  int `json:"still_open"` // window runs past the end of the data

	NetTotalPercent float64 `json:"net_total_percent"` // sum of ROE over all closed trades
	NetAvgPercent   float64 `json:"net_avg_percent"`
	WinRatePercent  float64 `json:"win_rate_percent"` // share of closed trades with ROE > 0
	AvgHoldDays     float64 `json:"avg_hold_days"`
	BestPercent     float64 `json:"best_percent"`
	WorstPercent    float64 `json:"worst_percent"`

	Trades []Trade `json:"trades"`
}

// Result is a whole run.
type Result struct {
	Params Params `json:"params"`
	From   int64  `json:"from"`
	To     int64  `json:"to"`
	// Step is the bucket size in minutes the samples were read at. A backtest
	// can only see the spread at this resolution, so it bounds how precisely an
	// exit can be timed.
	Step  int          `json:"step"`
	Pairs []PairResult `json:"pairs"`

	Entries         int     `json:"entries"`
	Exits           int     `json:"exits"`
	Timeouts        int     `json:"timeouts"`
	NoExitData      int     `json:"no_exit_data"`
	StillOpen       int     `json:"still_open"`
	NetTotalPercent float64 `json:"net_total_percent"`
	NetAvgPercent   float64 `json:"net_avg_percent"`
	WinRatePercent  float64 `json:"win_rate_percent"`
}

// RunPair simulates one Go/Return pair. goSamples and retSamples must be sorted
// by time; they are the no-fee series of goPath and its reverse.
//
// Only one position is held at a time: after a close, scanning resumes at the
// first go sample strictly after the exit. That is deliberately pessimistic
// about opportunity count and honest about capital - two overlapping positions
// would need twice the equity.
func RunPair(goPath string, goSamples, retSamples []Sample, p Params) PairResult {
	retPath := arbitrage.ReversePath(goPath)
	res := PairResult{Path: goPath, ReversePath: retPath, Trades: []Trade{}}

	goFeeMul := p.feeMultiplier(goPath)
	retFeeMul := p.feeMultiplier(retPath)
	window := int64(p.MaxHoldDays * 86400)

	// Where the recorded data stops. A position opened close to it cannot be
	// held for the full window, so it is reported as still open rather than
	// force-closed at a price that only looks final because we stopped looking.
	var dataEnd int64
	if len(retSamples) > 0 {
		dataEnd = retSamples[len(retSamples)-1].T
	}

	for i := 0; i < len(goSamples); i++ {
		entry := goSamples[i]
		g := withFees(entry.NoFeePercent, goFeeMul)
		if g < p.MinEntryPercent {
			continue
		}

		res.Entries++
		trade := Trade{
			EntryT:       entry.T,
			EntryPercent: g,
			Outcome:      OutcomeNoExitData,
		}

		// Candidate exits: return-leg samples strictly after entry, within the
		// holding window.
		lo := sort.Search(len(retSamples), func(k int) bool { return retSamples[k].T > entry.T })
		var lastExit Trade
		for k := lo; k < len(retSamples) && retSamples[k].T-entry.T <= window; k++ {
			s := retSamples[k]
			days := float64(s.T-entry.T) / 86400
			r := withFees(s.NoFeePercent, retFeeMul)
			rt := (1+g/100)*(1+r/100) - 1
			roe := p.roePercent(rt, days)

			lastExit = Trade{
				EntryT:           entry.T,
				ExitT:            s.T,
				HoldDays:         days,
				EntryPercent:     g,
				ExitPercent:      r,
				RoundTripPercent: rt * 100,
				NetROEPercent:    roe,
				Outcome:          OutcomeTimeout,
			}
			if roe >= p.TargetPercent {
				lastExit.Outcome = OutcomeTarget
				break
			}
		}

		if lastExit.ExitT != 0 {
			trade = lastExit
		}
		// dataEnd == 0 means the return leg has no history at all, which is a
		// missing leg rather than a window that ran out.
		if trade.Outcome != OutcomeTarget && dataEnd > 0 && entry.T+window > dataEnd {
			trade.Outcome = OutcomeOpen
		}

		switch trade.Outcome {
		case OutcomeTarget:
			res.Exits++
		case OutcomeTimeout:
			res.Timeouts++
		case OutcomeOpen:
			res.StillOpen++
		default:
			res.NoExitData++
		}
		res.Trades = append(res.Trades, trade)

		if trade.Outcome == OutcomeOpen {
			// The position never closed, so no later entry was fundable.
			break
		}
		// No overlapping positions: resume after this trade closed. A trade with
		// no exit data blocks nothing, so it only consumes its own sample.
		if trade.ExitT > 0 {
			for i+1 < len(goSamples) && goSamples[i+1].T <= trade.ExitT {
				i++
			}
		}
	}

	summarise(&res)
	return res
}

func summarise(res *PairResult) {
	var (
		closed  int
		wins    int
		sumROE  float64
		sumDays float64
		best    = math.Inf(-1)
		worst   = math.Inf(1)
	)
	for _, t := range res.Trades {
		if t.Outcome == OutcomeNoExitData || t.Outcome == OutcomeOpen {
			continue
		}
		closed++
		sumROE += t.NetROEPercent
		sumDays += t.HoldDays
		if t.NetROEPercent > 0 {
			wins++
		}
		best = math.Max(best, t.NetROEPercent)
		worst = math.Min(worst, t.NetROEPercent)
	}
	res.NetTotalPercent = sumROE
	if closed > 0 {
		res.NetAvgPercent = sumROE / float64(closed)
		res.AvgHoldDays = sumDays / float64(closed)
		res.WinRatePercent = float64(wins) / float64(closed) * 100
		res.BestPercent = best
		res.WorstPercent = worst
	}
}

// Run simulates every pair and aggregates. series maps a chain path to its
// no-fee samples; pairs that have only one direction recorded are reported with
// every entry counted as no_exit_data rather than silently dropped.
func Run(series map[string][]Sample, from, to int64, p Params) Result {
	res := Result{Params: p, From: from, To: to, Pairs: []PairResult{}}

	seen := make(map[string]bool, len(series))
	paths := make([]string, 0, len(series))
	for path := range series {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		goPath, retPath := arbitrage.GoAndReturn(path)
		if goPath == retPath || seen[goPath] {
			continue // a palindrome has no distinct return leg
		}
		seen[goPath] = true
		res.Pairs = append(res.Pairs, RunPair(goPath, series[goPath], series[retPath], p))
	}

	var closed, wins int
	for _, pr := range res.Pairs {
		res.Entries += pr.Entries
		res.Exits += pr.Exits
		res.Timeouts += pr.Timeouts
		res.NoExitData += pr.NoExitData
		res.StillOpen += pr.StillOpen
		res.NetTotalPercent += pr.NetTotalPercent
		for _, t := range pr.Trades {
			if t.Outcome == OutcomeNoExitData || t.Outcome == OutcomeOpen {
				continue
			}
			closed++
			if t.NetROEPercent > 0 {
				wins++
			}
		}
	}
	if closed > 0 {
		res.NetAvgPercent = res.NetTotalPercent / float64(closed)
		res.WinRatePercent = float64(wins) / float64(closed) * 100
	}

	sort.Slice(res.Pairs, func(i, j int) bool {
		return res.Pairs[i].NetTotalPercent > res.Pairs[j].NetTotalPercent
	})
	return res
}
