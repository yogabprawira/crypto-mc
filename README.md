# crypto-mc — most liquid crypto assets

A small Go web app that crawls CoinMarketCap with [colly v2](https://github.com/gocolly/colly),
ranks assets by **24h trading volume**, and excludes stablecoins.

- Web UI at `/` with sortable/filterable table
- **"Crawl again"** button for an on-demand crawl
- Background **scheduler** driven by an env-var interval

## Run

```bash
go mod download
go run .              # http://localhost:8080
```

Or build:

```bash
go build -o crypto-mc .
./crypto-mc
```

## Configuration

All settings are environment variables:

| Variable          | Default | Description |
|-------------------|---------|-------------|
| `PORT`            | `8080`  | HTTP listen port |
| `SCRAPE_INTERVAL` | `15m`   | Scheduler interval. Go duration (`30s`, `15m`, `1h30m`) or bare seconds (`900`). Set `0` to disable the scheduler. |
| `TOP_N`           | `200`   | How many assets to keep after filtering. The crawler requests `2 × TOP_N` (capped at 1000) so there is headroom for the excluded rows. |
| `SCRAPE_TIMEOUT`  | `45s`   | Per-request timeout |
| `SCRAPE_ON_START` | `true`  | Crawl once at startup |

Example:

```bash
PORT=9000 SCRAPE_INTERVAL=5m TOP_N=50 go run .
```

On Windows PowerShell:

```powershell
$env:PORT="9000"; $env:SCRAPE_INTERVAL="5m"; $env:TOP_N="50"; go run .
```

## Endpoints

| Method | Path           | Description |
|--------|----------------|-------------|
| `GET`  | `/`            | Web UI |
| `GET`  | `/api/assets`  | Latest snapshot (JSON) |
| `POST` | `/api/refresh` | Trigger a crawl now, returns the new snapshot |
| `GET`  | `/api/config`  | Interval / topN, used by the UI countdown |
| `GET`  | `/healthz`     | Liveness probe |

## Data source — important note

`https://coinmarketcap.com/all/views/all/` only **server-renders its first 20 rows**
(ranked by market cap). Rows 21+ arrive as empty skeletons that CoinMarketCap fills in
with client-side JavaScript. Colly does not execute JS, so scraping that HTML yields at
most 20 market-cap-ranked coins — not enough to rank by liquidity.

So the crawler uses two sources, both fetched with colly:

1. **Primary** — `api.coinmarketcap.com/data-api/v3/cryptocurrency/listing`, the same
   JSON endpoint that page calls to hydrate its own table. It returns 200 assets already
   sorted by `volume_24h`, and includes CoinMarketCap's own `stablecoin` tags.
2. **Fallback** — if that request fails, colly scrapes the HTML table at
   `/all/views/all/` and uses whatever rows are hydrated (~20).

The UI shows which source produced the current snapshot.

## Stablecoin exclusion

Two layers, in order:

1. CoinMarketCap's own tags — anything tagged `*stablecoin*`
   (`fiat-stablecoin`, `usd-stablecoin`, `asset-backed-stablecoin`, …).
2. A local check in `stablecoin.go`: a curated symbol list (`USDT`, `USDC`, `DAI`, …)
   plus a heuristic that only fires when a peg-flavoured name **and** a price within
   3% of $1 **and** a <1% 24h move all coincide — so a volatile token with "USD" in
   its name is not dropped by accident.

Layer 2 is what protects the HTML fallback path, which has no tags.

## Derivative exclusion

A wrapped, bridged, receipt or staked token is a claim on another asset, so its
volume double-counts the underlying coin's liquidity. `derivative.go` drops them
via two tags and three name rules:

| Rule | Catches |
|------|---------|
| tag `wrapped-tokens` | WETH, WBNB, WBTC, cbBTC |
| tag `rehypothecated-crypto` | stETH, JITOSOL, BTCB, plus the wrapped set |
| name contains `wrapped` | Wrapped Solana, WHYPE, WNEAR, WAVAX |
| name contains `staked` | Lido Staked ETH, Jito Staked SOL |
| name starts with `venus ` | vBNB, vETH |

Two traps this avoids:

- **`liquid-staking-derivatives` is the wrong tag.** CoinMarketCap also applies
  it to `LDO` (Lido DAO), `JTO` (Jito) and `BANK` (Lorenzo Protocol) — governance
  tokens that are genuine standalone assets. Filtering on it would delete all
  three. `rehypothecated-crypto` hits the receipts without touching them.
- **Symbol prefixes are unsafe.** `WLD`, `WBT`, `WIF`, `W` and `WXT` are real
  coins, so a `W*` rule would drop them. Detection uses tags and names only.

The `venus ` rule keeps its trailing space so the Venus governance token itself
(`XVS`, name exactly `Venus`) survives.

`TestIsDerivative` pins both directions, and a full `TOP_N=200` run confirms
LDO (#78), JTO (#88), BANK (#135), WIF, W and WXT are all still ranked while no
wrapped/staked/bridged token survives.

## Tokenized real-world asset exclusion

Tokenised equities, ETFs and commodities track an off-chain instrument rather
than a crypto asset, so `tokenized.go` drops them on any of these tags:

`tokenized-assets` · `tokenized-stock` · `tokenized-etfs` ·
`tokenized-commodities` · `tokenized-gold`

`tokenized-assets` is the umbrella and does the heavy lifting — over 100 tokens
carry it once you look past the top few hundred by volume (NVDA, AAPLB, SPY,
METAB, HOODX …), so tag matching scales where an issuer-name list would not.
A name fallback (`tokenized stock`, `tokenized etf`, `tokenized bstocks`,
`(xstock)`) covers the tag-less HTML path.

Three neighbouring signals are deliberately **not** used:

- **`real-world-assets-protocols`** tags protocols that work *with* RWAs, not
  tokenised assets themselves. It covers Chainlink, Stellar, Avalanche, Hedera,
  Algorand, VeChain, IOTA, Injective, Quant and Ondo — filtering on it would
  delete a large slice of genuine assets.
- **`stock-memes`** is for meme coins referencing equities. Ordinary tokens.
- **The word "gold" or "metal" in a name.** Adventure Gold (AGLD), Metal DAO
  (MTL) and Metal Dollar (XMD) are ordinary crypto tokens; only the tags tell
  them apart from Tether Gold and PAX Gold.

`TestIsTokenizedAsset` asserts all ten RWA protocols, the three metal-named
tokens and a stock-meme coin survive. A live run confirms LINK (#14), XLM
(#22), AVAX (#23) and ONDO (#29) are still ranked while XAUt, PAXG and every
tokenised equity are gone.

## Concurrency

The manual button and the scheduler share a single-flight guard (`store.go`): a refresh
requested while one is already running joins the in-flight crawl rather than starting a
second one. If a crawl fails outright, the previous good snapshot stays on screen and
the error is surfaced in a banner.

## Tests

```bash
go test ./...           # includes a live check of the HTML fallback
go test ./... -short    # unit tests only, no network
```

Frontend logic has its own harness. It extracts the functions from
`web/index.html` itself (so it cannot drift from what the browser runs) and
exercises them against a live snapshot — formatters, row rendering, the stat
tiles, volume-bar ordering, and the escaping described below:

```bash
go run .                    # in one shell
node web/ui_check.js 8080   # in another; pass the port you used
```

## Output escaping

Coin names come from a public listing that anyone can submit a token to, so the
table treats every scraped string as untrusted: text goes through `esc()` before
reaching `innerHTML`, and coin links go through `safeURL()`, which drops any URL
that is not `https://coinmarketcap.com/...` (so a `javascript:` href cannot be
injected through a crafted slug). `ui_check.js` asserts this with a hostile
payload.
