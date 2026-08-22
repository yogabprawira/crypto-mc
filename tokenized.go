package main

import "strings"

// isTokenizedAsset reports whether an asset is a tokenised real-world
// instrument — an on-chain wrapper around an equity, ETF or commodity (Tesla,
// Alphabet, SPY, QQQ, gold …). Its volume tracks an off-chain instrument
// rather than a crypto asset, so it is excluded from the liquidity ranking.
//
// Three neighbouring signals are deliberately NOT used:
//
//   - "real-world-assets-protocols" tags the protocols that work with RWAs,
//     not the tokenised assets themselves. It covers Chainlink, Stellar,
//     Avalanche, Hedera, Algorand, VeChain, IOTA, Injective, Quant and Ondo —
//     filtering on it would delete a large slice of genuine assets.
//   - "stock-memes" is for meme coins referencing equities (e.g. MARSCOIN).
//   - The word "gold" in a name. Adventure Gold (AGLD), Metal DAO (MTL) and
//     Metal Dollar (XMD) are ordinary crypto tokens; only the tags below
//     distinguish them from Tether Gold and PAX Gold.
func isTokenizedAsset(a Asset, tags []string) bool {
	for _, t := range tags {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "tokenized-assets", // the umbrella tag: equities, ETFs, commodities
			"tokenized-stock",
			"tokenized-etfs",
			"tokenized-commodities",
			"tokenized-gold":
			return true
		}
	}

	// Tag-less HTML fallback path. Listed tokenised instruments carry their
	// issuer's branding in the name: "… Tokenized bStocks" (Backed),
	// "… tokenized stock (xStock)" (xStocks), "… Tokenized Stock/ETF (Ondo)".
	name := strings.ToLower(strings.TrimSpace(a.Name))
	for _, marker := range []string{
		"tokenized stock", "tokenized etf", "tokenized bstocks", "(xstock)",
	} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}
