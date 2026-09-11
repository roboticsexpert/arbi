package mt5

// SymbolSpec maps a MetaTrader symbol onto the normalised pair the rest of the
// system speaks, plus the divisor that converts the broker's quote into one
// unit of Base.
type SymbolSpec struct {
	Base    string
	Quote   string
	Divisor float64
	// Volume is a placeholder size, in units of Base. MetaTrader publishes
	// top-of-book without depth unless the broker enables Depth of Market, so
	// there is no real quantity to report. Replace these with the symbol's
	// actual contract size and lot limits before treating MT5 as a tradeable
	// leg rather than a reference price.
	Volume string
}

// defaultSymbolMap covers the metals exposed by InternationalTrading-Server.
//
// GOLD_kilogram is preferred over GOLD_gram as the gram-price feed: the broker
// quotes GOLD_gram with only 2 decimal digits, which rounds the bid/ask spread
// away entirely (both sides print 141.21). The kilogram symbol carries the same
// price with enough precision to keep the spread intact, so we divide it by
// 1000 instead. Mapping GOLD_gram here as well would silently overwrite that
// with the rounded version, so it is deliberately absent.
var defaultSymbolMap = map[string]SymbolSpec{
	// 1 gram of 24k gold, in USD.
	"GOLD_kilogram": {Base: "GOLD24", Quote: "USD", Divisor: 1000, Volume: "1000"},
	// 1 troy ounce of gold, in USD. Kept as a separate pair so it can be used
	// as a sanity check against GOLD24 rather than competing with it.
	"GOLD_ounce": {Base: "XAU", Quote: "USD", Divisor: 1, Volume: "32"},
	// 1 troy ounce of silver, in USD.
	"SILVER_ounce": {Base: "XAG", Quote: "USD", Divisor: 1, Volume: "1000"},
}

// LookupSymbol returns the spec for a MetaTrader symbol name.
func LookupSymbol(symbol string) (SymbolSpec, bool) {
	spec, ok := defaultSymbolMap[symbol]
	return spec, ok
}

// KnownSymbols lists the MetaTrader symbols the ingest endpoint accepts.
func KnownSymbols() []string {
	out := make([]string, 0, len(defaultSymbolMap))
	for s := range defaultSymbolMap {
		out = append(out, s)
	}
	return out
}
