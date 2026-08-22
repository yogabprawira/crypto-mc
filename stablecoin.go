package main

import (
	"math"
	"strings"
)

// knownStables is a safety net for the HTML fallback path (which has no tags)
// and for anything CoinMarketCap has not tagged yet. Match is on the exact
// upper-cased symbol.
var knownStables = map[string]bool{
	"USDT": true, "USDC": true, "USDE": true, "DAI": true, "FDUSD": true,
	"PYUSD": true, "TUSD": true, "USDD": true, "USDS": true, "USD1": true,
	"BUSD": true, "GUSD": true, "LUSD": true, "FRAX": true, "USDP": true,
	"SUSD": true, "USDX": true, "USDY": true, "USDB": true, "USDF": true,
	"CRVUSD": true, "GHO": true, "MIM": true, "DOLA": true, "RLUSD": true,
	"EURC": true, "EURS": true, "EURT": true, "AEUR": true, "USTC": true,
	"SUSDE": true, "SUSDS": true, "USDG": true, "USDL": true, "USDO": true,
	"USDR": true, "USDQ": true, "USD0": true, "DEUSD": true, "LVLUSD": true,
	"USDA": true, "AUSD": true, "CGUSD": true, "FXUSD": true, "USDH": true,
}

// pegNameHints are substrings that, combined with a price sitting on a peg,
// identify a stablecoin whose ticker is not in the list above.
var pegNameHints = []string{
	"usd", "stable", "dollar", "euro", "tether", "dai", "peg",
}

// isStablecoin reports whether an asset is a fiat-pegged stablecoin, which we
// exclude from the liquidity ranking because their volume is transactional
// rather than a signal of speculative liquidity.
func isStablecoin(a Asset) bool {
	sym := strings.ToUpper(strings.TrimSpace(a.Symbol))
	if knownStables[sym] {
		return true
	}

	// Heuristic fallback for tokens not in the curated list: a peg-flavoured
	// name AND a price that actually sits on the $1 peg AND a flat 24h move.
	// All three must hold, so e.g. "Uniswap" or a volatile "USD"-named token
	// is not swept up by accident.
	name := strings.ToLower(a.Name)
	hinted := false
	for _, h := range pegNameHints {
		if strings.Contains(name, h) || strings.Contains(strings.ToLower(sym), h) {
			hinted = true
			break
		}
	}
	if !hinted {
		return false
	}
	onPeg := a.Price > 0.97 && a.Price < 1.03
	flat := math.Abs(a.Change24h) < 1.0
	return onPeg && flat
}
