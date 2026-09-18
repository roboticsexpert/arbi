package history

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"arbi/internal/arbitrage"
)

func chain(path string, pct, noFee float64) arbitrage.ArbitrageChain {
	return arbitrage.ArbitrageChain{Path: path, ProfitPercent: pct, ProfitPercentNoFee: noFee}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRecordBucketsAndQueries(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "h.db")
	rec, err := Open(dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	const go_, ret = "IRT-a-X-b-IRT", "IRT-b-X-a-IRT"

	// minute 0: three samples of go, one of ret
	rec.Record(base.Add(5*time.Second), []arbitrage.ArbitrageChain{chain(go_, 0.1, 0.3), chain(ret, -0.5, -0.3)})
	rec.Record(base.Add(15*time.Second), []arbitrage.ArbitrageChain{chain(go_, 0.4, 0.6)})
	rec.Record(base.Add(25*time.Second), []arbitrage.ArbitrageChain{chain(go_, -0.2, 0.0)})
	// minute 1: one sample of go; minute 2 empty run rolls it over
	rec.Record(base.Add(65*time.Second), []arbitrage.ArbitrageChain{chain(go_, 1.0, 1.2)})
	rec.Record(base.Add(125*time.Second), nil)

	s, err := rec.Series([]string{go_, ret, "missing"}, base, base.Add(time.Hour), 1)
	if err != nil {
		t.Fatal(err)
	}
	g := s[go_]
	if len(g) != 2 {
		t.Fatalf("go points = %d, want 2: %+v", len(g), g)
	}
	if g[0].T != base.Unix() || g[0].Samples != 3 || !near(g[0].Avg, 0.1) ||
		g[0].Min != -0.2 || g[0].Max != 0.4 || g[0].Last != -0.2 || !near(g[0].AvgNoFee, 0.3) {
		t.Fatalf("minute 0 bucket wrong: %+v", g[0])
	}
	if len(s[ret]) != 1 || len(s["missing"]) != 0 {
		t.Fatalf("ret/missing wrong: %+v / %+v", s[ret], s["missing"])
	}

	// 5-minute step folds both minutes, weighted by samples: (0.1*3 + 1.0)/4
	s5, _ := rec.Series([]string{go_}, base, base.Add(time.Hour), 5)
	if p := s5[go_]; len(p) != 1 || !near(p[0].Avg, 0.325) || p[0].Last != 1.0 || p[0].Samples != 4 || p[0].Max != 1.0 {
		t.Fatalf("5m bucket wrong: %+v", s5[go_])
	}

	// A restart inside minute 2 merges with what Close flushed.
	rec.Record(base.Add(130*time.Second), []arbitrage.ArbitrageChain{chain(go_, 2.0, 2.0)})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	rec, err = Open(dbPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer rec.Close()
	rec.Record(base.Add(140*time.Second), []arbitrage.ArbitrageChain{chain(go_, 4.0, 4.0)})
	rec.Record(base.Add(185*time.Second), nil)

	s, _ = rec.Series([]string{go_}, base.Add(2*time.Minute), base.Add(3*time.Minute), 1)
	if p := s[go_]; len(p) != 1 || p[0].Samples != 2 || !near(p[0].Avg, 3.0) || p[0].Min != 2.0 || p[0].Last != 4.0 {
		t.Fatalf("merged minute wrong: %+v", s[go_])
	}

	paths, err := rec.Paths()
	if err != nil || len(paths) != 2 || paths[0].Path != go_ || paths[0].LastSeen != base.Add(2*time.Minute).Unix() {
		t.Fatalf("paths wrong: %+v %v", paths, err)
	}
}

func TestAutoStep(t *testing.T) {
	now := time.Now()
	for _, c := range []struct {
		d    time.Duration
		want int
	}{{time.Hour, 1}, {24 * time.Hour, 1}, {7 * 24 * time.Hour, 5}, {30 * 24 * time.Hour, 15}} {
		if got := AutoStep(now.Add(-c.d), now); got != c.want {
			t.Errorf("AutoStep(%s) = %d, want %d", c.d, got, c.want)
		}
	}
}
