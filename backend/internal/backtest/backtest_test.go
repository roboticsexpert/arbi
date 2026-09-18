package backtest

import (
	"math"
	"testing"

	"arbi/internal/arbitrage"
)

const goPath = "IRT-ecogold-GOLD18-convert-PAXG-kucoin-USDT-nobitex-IRT"

// testParams strips every cost but trading fees, so a test asserts on one thing
// at a time. Individual tests switch the ones they care about back on.
func testParams() Params {
	return Params{
		TargetPercent:   1.0,
		MinEntryPercent: 1.0,
		MaxHoldDays:     7,
		CapitalMult:     1.0,
		Fees:            map[string]float64{"ecogold": 0, "kucoin": 0, "nobitex": 0},
	}
}

func TestReversePathIsAnInvolution(t *testing.T) {
	rev := arbitrage.ReversePath(goPath)
	want := "IRT-nobitex-USDT-kucoin-PAXG-convert-GOLD18-ecogold-IRT"
	if rev != want {
		t.Fatalf("ReversePath = %q, want %q", rev, want)
	}
	if back := arbitrage.ReversePath(rev); back != goPath {
		t.Fatalf("reversing twice gave %q, want %q", back, goPath)
	}
}

func TestPathVenuesSkipsConversions(t *testing.T) {
	got := arbitrage.PathVenues(goPath)
	want := []string{"ecogold", "kucoin", "nobitex"}
	if len(got) != len(want) {
		t.Fatalf("PathVenues = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PathVenues = %v, want %v", got, want)
		}
	}
}

// A path's fee multiplier must compound one fee per trade hop and none for the
// conversion, otherwise re-costing history silently overcharges.
func TestFeeMultiplierCompoundsTradeHopsOnly(t *testing.T) {
	p := testParams()
	p.Fees = map[string]float64{"ecogold": 0, "kucoin": 0.001, "nobitex": 0.0025}

	want := 1 * (1 - 0.001) * (1 - 0.0025)
	if got := p.feeMultiplier(goPath); math.Abs(got-want) > 1e-12 {
		t.Fatalf("feeMultiplier = %v, want %v", got, want)
	}
}

func TestWithFeesMatchesFinderArithmetic(t *testing.T) {
	// The finder multiplies the running amount by (1-fee) at each hop, so a
	// chain that grew 2% before fees and paid 0.35% in total grows by
	// 1.02*0.9965 - 1.
	got := withFees(2, 0.9965)
	want := (1.02*0.9965 - 1) * 100
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("withFees = %v, want %v", got, want)
	}
}

func TestExitsAtFirstSampleClearingTarget(t *testing.T) {
	p := testParams()
	day := int64(86400)

	// Go leg sits at +2%. The return leg only clears the 1% target on day 2.
	got := RunPair(goPath,
		[]Sample{{T: 0, NoFeePercent: 2}},
		[]Sample{
			{T: day, NoFeePercent: -1.5}, // round trip 0.47% - short of target
			{T: 2 * day, NoFeePercent: 0.5},
		},
		p)

	if got.Entries != 1 || got.Exits != 1 || got.Timeouts != 0 {
		t.Fatalf("entries=%d exits=%d timeouts=%d, want 1/1/0", got.Entries, got.Exits, got.Timeouts)
	}
	tr := got.Trades[0]
	if tr.Outcome != OutcomeTarget {
		t.Fatalf("outcome = %q, want %q", tr.Outcome, OutcomeTarget)
	}
	if tr.ExitT != 2*day {
		t.Fatalf("exited at %d, want %d - it must not take the first sample that is still short of target", tr.ExitT, 2*day)
	}
	wantRT := (1.02*1.005 - 1) * 100
	if math.Abs(tr.RoundTripPercent-wantRT) > 1e-9 {
		t.Fatalf("round trip = %v, want %v", tr.RoundTripPercent, wantRT)
	}
}

// A position that never reaches the target is force-closed at the last sample
// inside the window and books whatever that is worth - usually a loss. Counting
// it as "no trade" would flatter every result.
func TestTimeoutForceClosesAtLastSampleInWindow(t *testing.T) {
	p := testParams()
	p.MaxHoldDays = 2
	day := int64(86400)

	got := RunPair(goPath,
		[]Sample{{T: 0, NoFeePercent: 2}},
		[]Sample{
			{T: day, NoFeePercent: -3},
			{T: 2 * day, NoFeePercent: -2.5},
			{T: 3 * day, NoFeePercent: 5}, // outside the window
		},
		p)

	if got.Timeouts != 1 || got.Exits != 0 {
		t.Fatalf("exits=%d timeouts=%d, want 0/1", got.Exits, got.Timeouts)
	}
	tr := got.Trades[0]
	if tr.ExitT != 2*day {
		t.Fatalf("exited at %d, want %d (the last sample within MaxHoldDays)", tr.ExitT, 2*day)
	}
	if tr.NetROEPercent >= 0 {
		t.Fatalf("net ROE = %v, want a loss", tr.NetROEPercent)
	}
}

func TestNoReturnLegDataIsReportedNotDropped(t *testing.T) {
	got := RunPair(goPath, []Sample{{T: 0, NoFeePercent: 2}}, nil, testParams())

	if got.Entries != 1 || got.NoExitData != 1 {
		t.Fatalf("entries=%d no_exit_data=%d, want 1/1", got.Entries, got.NoExitData)
	}
	if got.Trades[0].Outcome != OutcomeNoExitData {
		t.Fatalf("outcome = %q, want %q", got.Trades[0].Outcome, OutcomeNoExitData)
	}
	// A trade with no exit is not a closed trade, so it must not move the stats.
	if got.NetTotalPercent != 0 || got.WinRatePercent != 0 {
		t.Fatalf("unclosed trade leaked into stats: net=%v winrate=%v", got.NetTotalPercent, got.WinRatePercent)
	}
}

// Positions must not overlap: the equity is committed until the position closes,
// so a second entry before that exit is not fundable.
func TestPositionsDoNotOverlap(t *testing.T) {
	p := testParams()
	hour := int64(3600)

	got := RunPair(goPath,
		[]Sample{
			{T: 0, NoFeePercent: 2},
			{T: hour, NoFeePercent: 2},     // inside the first position
			{T: 2 * hour, NoFeePercent: 2}, // still inside
			{T: 4 * hour, NoFeePercent: 2}, // after it closed
		},
		[]Sample{
			{T: 3 * hour, NoFeePercent: 0.5},
			{T: 5 * hour, NoFeePercent: 0.5},
		},
		p)

	if got.Entries != 2 {
		t.Fatalf("entries = %d, want 2 - entries during an open position must be skipped", got.Entries)
	}
	if got.Trades[0].ExitT != 3*hour || got.Trades[1].EntryT != 4*hour {
		t.Fatalf("trades = %+v, want the second to start after the first closed", got.Trades)
	}
}

func TestCarryAndProfitShareReduceReturn(t *testing.T) {
	day := int64(86400)
	samples := func() ([]Sample, []Sample) {
		return []Sample{{T: 0, NoFeePercent: 2}}, []Sample{{T: 6 * day, NoFeePercent: 0.5}}
	}

	free := testParams()
	free.TargetPercent = 100 // never exit early; force the timeout mark-to-market
	g, r := samples()
	base := RunPair(goPath, g, r, free).Trades[0].NetROEPercent

	costly := free
	costly.CarryPerDayPercent = 0.15
	g, r = samples()
	carried := RunPair(goPath, g, r, costly).Trades[0].NetROEPercent

	if wantDrop := 0.15 * 6; math.Abs((base-carried)-wantDrop) > 1e-9 {
		t.Fatalf("carry cost %v over 6 days, want %v", base-carried, wantDrop)
	}

	shared := costly
	shared.ProfitSharePerDayPercent = 0.5
	g, r = samples()
	afterShare := RunPair(goPath, g, r, shared).Trades[0].NetROEPercent

	// 6 days held = 5 extension days = 2.5% of the profit.
	if want := carried * (1 - 0.005*5); math.Abs(afterShare-want) > 1e-9 {
		t.Fatalf("after profit share = %v, want %v", afterShare, want)
	}
}

// A loss must not be shrunk by the profit share - Nobitex takes nothing when the
// position is not profitable.
func TestProfitShareDoesNotApplyToLosses(t *testing.T) {
	p := testParams()
	p.TargetPercent = 100
	p.ProfitSharePerDayPercent = 0.5
	day := int64(86400)

	tr := RunPair(goPath,
		[]Sample{{T: 0, NoFeePercent: 2}},
		[]Sample{{T: 6 * day, NoFeePercent: -5}},
		p).Trades[0]

	wantRT := (1.02*0.95 - 1) * 100
	if tr.NetROEPercent >= 0 {
		t.Fatalf("net ROE = %v, want a loss", tr.NetROEPercent)
	}
	if math.Abs(tr.NetROEPercent-wantRT) > 1e-9 {
		t.Fatalf("net ROE = %v, want the untouched round trip %v", tr.NetROEPercent, wantRT)
	}
}

func TestRequiredRoundTripMatchesDocumentedExample(t *testing.T) {
	// docs/trade-economics.md §5, Structure A: K=0.83, carry 0.15%/day,
	// X=0.04%, p=0.5%/day, T=1%.
	p := DefaultParams()

	if got, want := p.RequiredRoundTripPercent(1), 1.39; math.Abs(got-want) > 0.01 {
		t.Fatalf("RequiredRoundTripPercent(1) = %.3f, want %.2f", got, want)
	}
	if got, want := p.RequiredRoundTripPercent(7), 2.33; math.Abs(got-want) > 0.01 {
		t.Fatalf("RequiredRoundTripPercent(7) = %.3f, want %.2f", got, want)
	}
}

func TestRunSkipsPalindromesAndPairsBothDirections(t *testing.T) {
	rev := arbitrage.ReversePath(goPath)
	series := map[string][]Sample{
		goPath:            {{T: 0, NoFeePercent: 2}},
		rev:               {{T: 86400, NoFeePercent: 0.5}},
		"IRT-nobitex-IRT": {{T: 0, NoFeePercent: 9}}, // its own reverse
	}

	got := Run(series, 0, 2*86400, testParams())

	if len(got.Pairs) != 1 {
		t.Fatalf("got %d pairs, want 1 (the palindrome must be skipped)", len(got.Pairs))
	}
	if got.Entries != 1 || got.Exits != 1 {
		t.Fatalf("entries=%d exits=%d, want 1/1", got.Entries, got.Exits)
	}
}

// A position opened near the end of the recorded data has not "timed out" -
// there simply is no more data. Booking it as a loss would make every run look
// worse in its most recent MaxHoldDays.
func TestPositionPastEndOfDataIsStillOpenNotATimeout(t *testing.T) {
	p := testParams()
	p.MaxHoldDays = 7
	day := int64(86400)

	got := RunPair(goPath,
		[]Sample{{T: 0, NoFeePercent: 2}},
		// Data stops two days after entry, well inside the 7-day window.
		[]Sample{{T: day, NoFeePercent: -3}, {T: 2 * day, NoFeePercent: -2.5}},
		p)

	if got.StillOpen != 1 || got.Timeouts != 0 {
		t.Fatalf("still_open=%d timeouts=%d, want 1/0", got.StillOpen, got.Timeouts)
	}
	if got.Trades[0].Outcome != OutcomeOpen {
		t.Fatalf("outcome = %q, want %q", got.Trades[0].Outcome, OutcomeOpen)
	}
	// An unresolved position must not book a loss into the totals.
	if got.NetTotalPercent != 0 || got.WinRatePercent != 0 {
		t.Fatalf("unresolved position leaked into stats: net=%v winrate=%v", got.NetTotalPercent, got.WinRatePercent)
	}
}

// Once a position is left open at the end of the data, no later entry could
// have been funded, so scanning must stop rather than opening a second one.
func TestStillOpenPositionBlocksLaterEntries(t *testing.T) {
	p := testParams()
	p.MaxHoldDays = 7
	day := int64(86400)

	got := RunPair(goPath,
		[]Sample{{T: 0, NoFeePercent: 2}, {T: 3 * day, NoFeePercent: 2}},
		[]Sample{{T: day, NoFeePercent: -3}},
		p)

	if got.Entries != 1 {
		t.Fatalf("entries = %d, want 1", got.Entries)
	}
}
