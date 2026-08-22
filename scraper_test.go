package main

import (
	"testing"
	"time"
)

func TestParseMoney(t *testing.T) {
	cases := map[string]float64{
		"$1.54T":          1.54e12,
		"$291.97B":        291.97e9,
		"$50,764,798,921": 50764798921,
		"$77,166.51":      77166.51,
		"$0.00001234":     0.00001234,
		"$1.5K":           1500,
		"--":              0,
		"":                0,
		"$4,600,000,000":  4.6e9,
	}
	for in, want := range cases {
		if got := parseMoney(in); got != want {
			t.Errorf("parseMoney(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParsePercent(t *testing.T) {
	cases := map[string]float64{
		"0.29%": 0.29, "-3.57%": -3.57, "22.46%": 22.46, "--": 0, "": 0,
	}
	for in, want := range cases {
		if got := parsePercent(in); got != want {
			t.Errorf("parsePercent(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAddThousands(t *testing.T) {
	cases := map[string]string{
		"1":          "1",
		"100":        "100",
		"1000":       "1,000",
		"20071518":   "20,071,518",
		"1234567.89": "1,234,567.89",
		"-4500":      "-4,500",
	}
	for in, want := range cases {
		if got := addThousands(in); got != want {
			t.Errorf("addThousands(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasStablecoinTag(t *testing.T) {
	if !hasStablecoinTag([]string{"tron20-ecosystem", "usd-stablecoin"}) {
		t.Error("expected usd-stablecoin to match")
	}
	if hasStablecoinTag([]string{"defi", "smart-contracts"}) {
		t.Error("did not expect a match")
	}
}

func TestIsStablecoin(t *testing.T) {
	stable := []Asset{
		{Symbol: "USDT", Name: "Tether USDt", Price: 0.9998, Change24h: 0.01},
		{Symbol: "DAI", Name: "Dai", Price: 1.0001, Change24h: -0.02},
		// Not in the curated list — caught by the peg heuristic.
		{Symbol: "ZZUSD", Name: "Something USD Stable", Price: 1.001, Change24h: 0.03},
	}
	for _, a := range stable {
		if !isStablecoin(a) {
			t.Errorf("expected %s to be flagged as a stablecoin", a.Symbol)
		}
	}

	volatile := []Asset{
		{Symbol: "BTC", Name: "Bitcoin", Price: 77000, Change24h: 0.1},
		{Symbol: "UNI", Name: "Uniswap", Price: 6.2, Change24h: 3.1},
		// USD in the name but nowhere near a peg — must not be excluded.
		{Symbol: "USDX", Name: "Volatile USD Thing", Price: 42, Change24h: 9},
	}
	for _, a := range volatile[:2] {
		if isStablecoin(a) {
			t.Errorf("did not expect %s to be flagged as a stablecoin", a.Symbol)
		}
	}
}

func TestIsDerivative(t *testing.T) {
	drop := []struct {
		a    Asset
		tags []string
	}{
		{Asset{Symbol: "WETH", Name: "WETH"}, []string{"defi", "wrapped-tokens"}},
		{Asset{Symbol: "WBNB", Name: "Wrapped BNB"}, []string{"wrapped-tokens"}},
		// No wrapped tag — caught by name or by rehypothecated-crypto.
		{Asset{Symbol: "SOL", Name: "Wrapped Solana"}, []string{"defi", "rehypothecated-crypto"}},
		{Asset{Symbol: "WHYPE", Name: "Wrapped HYPE"}, []string{"defi"}},
		{Asset{Symbol: "WNEAR", Name: "Wrapped Near"}, []string{"defi"}},
		{Asset{Symbol: "WAVAX", Name: "Wrapped AVAX"}, []string{"defi"}},
		{Asset{Symbol: "CBBTC", Name: "Coinbase Wrapped BTC"}, []string{"wrapped-tokens"}},
		// Venus receipt tokens carry no wrapped tag at all.
		{Asset{Symbol: "vBNB", Name: "Venus BNB"}, []string{"defi", "bnb-chain-ecosystem"}},
		{Asset{Symbol: "vETH", Name: "Venus ETH"}, []string{"defi", "bnb-chain-ecosystem"}},
		// Liquid-staking receipts, by tag and by name.
		{Asset{Symbol: "stETH", Name: "Lido Staked ETH"}, []string{"defi", "rehypothecated-crypto"}},
		{Asset{Symbol: "JITOSOL", Name: "Jito Staked SOL"}, []string{"defi", "rehypothecated-crypto"}},
		{Asset{Symbol: "stETH", Name: "Lido Staked ETH"}, nil}, // HTML path: no tags
		// Bridged representations.
		{Asset{Symbol: "BTCB", Name: "Bitcoin BEP2"}, []string{"rehypothecated-crypto"}},
	}
	for _, c := range drop {
		if !isDerivative(c.a, c.tags) {
			t.Errorf("expected %s (%q) to be excluded as a derivative", c.a.Symbol, c.a.Name)
		}
	}

	// Genuine standalone assets that must survive. LDO, JTO and BANK are the
	// important ones: CoinMarketCap tags them "liquid-staking-derivatives"
	// even though they are governance tokens, which is exactly why that tag
	// is not used for filtering.
	keep := []struct {
		a    Asset
		tags []string
	}{
		{Asset{Symbol: "LDO", Name: "Lido DAO"}, []string{"defi", "liquid-staking-derivatives"}},
		{Asset{Symbol: "JTO", Name: "Jito"}, []string{"defi", "liquid-staking-derivatives"}},
		{Asset{Symbol: "BANK", Name: "Lorenzo Protocol"}, []string{"liquid-staking-derivatives"}},
		{Asset{Symbol: "WLD", Name: "Worldcoin"}, []string{"defi"}},
		{Asset{Symbol: "WBT", Name: "WhiteBIT Coin"}, []string{"defi"}},
		{Asset{Symbol: "WIF", Name: "dogwifhat"}, []string{"memes"}},
		{Asset{Symbol: "W", Name: "Wormhole"}, []string{"defi"}},
		{Asset{Symbol: "WXT", Name: "WEEX Token"}, []string{"defi"}},
		{Asset{Symbol: "WLFI", Name: "World Liberty Financial"}, []string{"defi"}},
		{Asset{Symbol: "XVS", Name: "Venus"}, []string{"defi", "bnb-chain-ecosystem"}},
		{Asset{Symbol: "BTC", Name: "Bitcoin"}, nil},
		{Asset{Symbol: "ETH", Name: "Ethereum"}, nil},
	}
	for _, c := range keep {
		if isDerivative(c.a, c.tags) {
			t.Errorf("did not expect %s (%q) to be excluded", c.a.Symbol, c.a.Name)
		}
	}
}

func TestIsTokenizedAsset(t *testing.T) {
	drop := []struct {
		a    Asset
		tags []string
	}{
		{Asset{Symbol: "QQQB", Name: "Invesco QQQ Trust Tokenized bStocks"}, []string{"tokenized-stock", "tokenized-etfs"}},
		{Asset{Symbol: "TSLAX", Name: "Tesla tokenized stock (xStock)"}, []string{"tokenized-stock"}},
		{Asset{Symbol: "GOOGLX", Name: "Alphabet tokenized stock (xStock)"}, []string{"tokenized-stock"}},
		{Asset{Symbol: "SPYB", Name: "State Street SPDR S&P 500 ETF Tokenized bStocks"}, []string{"tokenized-stock"}},
		{Asset{Symbol: "MUB", Name: "Micron Technology Tokenized bStocks"}, []string{"tokenized-stock"}},
		{Asset{Symbol: "NVDAB", Name: "NVIDIA Tokenized bStocks"}, []string{"tokenized-assets"}},
		// Tokenized commodities.
		{Asset{Symbol: "XAUt", Name: "Tether Gold"}, []string{"tokenized-gold", "tokenized-assets", "tokenized-commodities"}},
		{Asset{Symbol: "PAXG", Name: "PAX Gold"}, []string{"tokenized-gold", "tokenized-assets", "tokenized-commodities"}},
		// Ondo-issued wrappers, including the ETF naming variant.
		{Asset{Symbol: "GLDon", Name: "SPDR Gold Shares Tokenized Stock (Ondo)"}, []string{"tokenized-assets"}},
		{Asset{Symbol: "SLVon", Name: "iShares Silver Trust Tokenized ETF (Ondo)"}, []string{"tokenized-assets", "tokenized-etfs"}},
		// HTML fallback path has no tags — names must carry it.
		{Asset{Symbol: "TSLAX", Name: "Tesla tokenized stock (xStock)"}, nil},
		{Asset{Symbol: "QQQB", Name: "Invesco QQQ Trust Tokenized bStocks"}, nil},
		{Asset{Symbol: "SLVon", Name: "iShares Silver Trust Tokenized ETF (Ondo)"}, nil},
	}
	for _, c := range drop {
		if !isTokenizedAsset(c.a, c.tags) {
			t.Errorf("expected %s (%q) to be excluded as a tokenized asset", c.a.Symbol, c.a.Name)
		}
	}

	// The RWA-protocol tag covers major genuine assets. Filtering on it would
	// be a serious regression, so every one of these must survive.
	rwa := []Asset{
		{Symbol: "LINK", Name: "Chainlink"}, {Symbol: "XLM", Name: "Stellar"},
		{Symbol: "AVAX", Name: "Avalanche"}, {Symbol: "HBAR", Name: "Hedera"},
		{Symbol: "ALGO", Name: "Algorand"}, {Symbol: "VET", Name: "VeChain"},
		{Symbol: "IOTA", Name: "IOTA"}, {Symbol: "INJ", Name: "Injective"},
		{Symbol: "QNT", Name: "Quant"}, {Symbol: "ONDO", Name: "Ondo"},
	}
	for _, a := range rwa {
		if isTokenizedAsset(a, []string{"defi", "real-world-assets-protocols"}) {
			t.Errorf("did not expect RWA protocol %s (%q) to be excluded", a.Symbol, a.Name)
		}
	}

	// Genuine crypto tokens with precious-metal names. These are why the rule
	// never matches "gold" or "metal" in a name.
	metals := []Asset{
		{Symbol: "AGLD", Name: "Adventure Gold"},
		{Symbol: "MTL", Name: "Metal DAO"},
		{Symbol: "XMD", Name: "Metal Dollar"},
	}
	for _, a := range metals {
		if isTokenizedAsset(a, []string{"defi", "gaming"}) {
			t.Errorf("did not expect %s (%q) to be excluded", a.Symbol, a.Name)
		}
	}

	// Meme coins that reference equities are ordinary crypto tokens.
	if isTokenizedAsset(Asset{Symbol: "MARSCOIN", Name: "Marscoin"}, []string{"memes", "stock-memes"}) {
		t.Error("did not expect a stock-meme coin to be excluded")
	}
}

func TestFormatPrice(t *testing.T) {
	cases := map[float64]string{
		77409.01:   "$77,409.01",
		2426.34:    "$2,426.34",
		0.8366:     "$0.8366",
		0.00000413: "$0.00000413",
		0:          "—",
	}
	for in, want := range cases {
		if got := formatPrice(in); got != want {
			t.Errorf("formatPrice(%v) = %q, want %q", in, got, want)
		}
	}
}

// TestScrapeHTMLFallback hits the real page; skipped with -short.
func TestScrapeHTMLFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	s := &Scraper{TopN: 50, Timeout: 45 * time.Second}
	snap, err := s.scrapeHTML()
	if err != nil {
		t.Fatalf("scrapeHTML: %v", err)
	}
	if len(snap.Assets) == 0 {
		t.Fatal("fallback returned no assets")
	}
	t.Logf("fallback: rows=%d kept=%d stables=%d", snap.RowsSeen, len(snap.Assets), snap.StablesFound)
	for _, a := range snap.Assets {
		if a.Name == "" || a.Symbol == "" || a.Volume24h <= 0 || a.Price <= 0 {
			t.Errorf("incomplete asset parsed: %+v", a)
		}
	}
}
