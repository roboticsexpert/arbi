package backtest_test

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"arbi/internal/arbitrage"
	"arbi/internal/backtest"
	"arbi/internal/history"
)

// The backtest reads avg_no_fee out of the history database, so the wiring from
// recorded chains through Recorder.Series to backtest.Sample has to survive the
// round trip. A unit test on Run cannot catch a mix-up between the with-fee and
// no-fee columns; this one can.
func TestBacktestOverRecordedHistory(t *testing.T) {
	rec, err := history.Open(filepath.Join(t.TempDir(), "history.db"), 0)
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer rec.Close()

	goPath := "IRT-ecogold-GOLD18-convert-PAXG-kucoin-USDT-nobitex-IRT"
	retPath := arbitrage.ReversePath(goPath)

	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	chain := func(path string, noFee float64) arbitrage.ArbitrageChain {
		return arbitrage.ArbitrageChain{
			Path: path,
			// The finder's own fee assumption lands in ProfitPercent. The
			// backtest must ignore it and re-cost from ProfitPercentNoFee.
			ProfitPercent:      noFee - 0.9,
			ProfitPercentNoFee: noFee,
		}
	}

	// The spread starts wide (go leg attractive, return leg deeply negative) and
	// converges over three days.
	for minute := 0; minute <= 3*24*60; minute += 10 {
		at := start.Add(time.Duration(minute) * time.Minute)
		progress := float64(minute) / float64(3*24*60)
		rec.Record(at, []arbitrage.ArbitrageChain{
			chain(goPath, 3),
			chain(retPath, -3+3.6*progress),
		})
	}
	// Recording one more sample in a later minute flushes the final bucket.
	rec.Record(start.Add(time.Duration(3*24*60+1)*time.Minute), nil)

	from, to := start.Add(-time.Minute), start.Add(4*24*time.Hour)
	series, err := rec.Series([]string{goPath, retPath}, from, to, 15)
	if err != nil {
		t.Fatalf("series: %v", err)
	}
	if len(series[goPath]) == 0 || len(series[retPath]) == 0 {
		t.Fatalf("no history came back: go=%d ret=%d", len(series[goPath]), len(series[retPath]))
	}

	samples := map[string][]backtest.Sample{}
	for path, points := range series {
		for _, pt := range points {
			samples[path] = append(samples[path], backtest.Sample{T: pt.T, NoFeePercent: pt.AvgNoFee})
		}
	}
	// The go leg is flat at 3% before fees; if the with-fee column leaked in
	// here it would read 2.1% instead.
	if got := samples[goPath][0].NoFeePercent; math.Abs(got-3) > 1e-6 {
		t.Fatalf("go leg sample = %v, want the no-fee 3 (the with-fee column is 2.1)", got)
	}

	p := backtest.DefaultParams()
	res := backtest.Run(samples, from.Unix(), to.Unix(), p)

	if len(res.Pairs) != 1 {
		t.Fatalf("got %d pairs, want 1", len(res.Pairs))
	}
	if res.Entries == 0 {
		t.Fatal("no entries: a go leg pinned at +3% must clear the 1% entry threshold")
	}
	if res.Exits == 0 {
		t.Fatal("no exits: a spread that converges by 3.6 points must reach the target")
	}

	first := res.Pairs[0].Trades[0]
	if first.Outcome != backtest.OutcomeTarget {
		t.Fatalf("first trade outcome = %q, want %q", first.Outcome, backtest.OutcomeTarget)
	}
	if first.HoldDays <= 0 || first.HoldDays > p.MaxHoldDays {
		t.Fatalf("hold = %v days, want within (0, %v]", first.HoldDays, p.MaxHoldDays)
	}
	if first.NetROEPercent < p.TargetPercent {
		t.Fatalf("closed at %v%%, below the %v%% target", first.NetROEPercent, p.TargetPercent)
	}
}
