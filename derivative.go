package main

import "strings"

// isDerivative reports whether an asset is a claim on some other asset rather
// than an asset in its own right — wrapped tokens, bridged tokens, protocol
// receipt tokens and liquid-staking derivatives. Their volume double-counts the
// underlying coin's liquidity, so they are excluded from the ranking.
//
// Detection never keys off the symbol prefix: plenty of genuine assets start
// with "W" (Worldcoin/WLD, WhiteBIT/WBT, dogwifhat/WIF, Wormhole/W, WEEX/WXT),
// so a "W*" rule would drop real coins.
func isDerivative(a Asset, tags []string) bool {
	for _, t := range tags {
		switch strings.ToLower(strings.TrimSpace(t)) {
		// Explicit wrapping. Reliable when present but incomplete — it covers
		// WETH/WBNB/WBTC/cbBTC yet misses Wrapped Solana, WHYPE, WNEAR, WAVAX.
		case "wrapped-tokens":
			return true

		// CoinMarketCap's marker for "this token represents another token".
		// Covers the wrapped set plus stETH, JITOSOL and bridged coins such as
		// BTCB, and — unlike "liquid-staking-derivatives" — does NOT sweep in
		// the governance tokens LDO, JTO and BANK, which are genuine assets.
		case "rehypothecated-crypto":
			return true
		}
	}

	name := strings.ToLower(strings.TrimSpace(a.Name))

	// The tag-less HTML fallback path relies entirely on these name rules.

	// "Wrapped Solana", "Wrapped BNB", "Coinbase Wrapped BTC", "Wrapped Near".
	if strings.Contains(name, "wrapped") {
		return true
	}

	// Liquid-staking receipts: "Lido Staked ETH", "Jito Staked SOL". The issuer
	// tokens ("Lido DAO", "Jito") do not contain "staked" and so survive.
	if strings.Contains(name, "staked") {
		return true
	}

	// Venus protocol receipt tokens: vBNB ("Venus BNB"), vETH ("Venus ETH").
	// The trailing space matters — it keeps the Venus governance token itself
	// (XVS, whose name is exactly "Venus") in the list.
	if strings.HasPrefix(name, "venus ") {
		return true
	}

	return false
}
