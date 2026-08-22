package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/extensions"
)

// listingURL is the JSON listing that the /all/views/all/ page itself calls to
// hydrate its table. The HTML at pageURL only server-renders the first 20 rows
// (the rest are JS-filled skeletons), so this is the only way a non-JS crawler
// can see the full set — and it carries CoinMarketCap's own "stablecoin" tag.
const listingURL = "https://api.coinmarketcap.com/data-api/v3/cryptocurrency/listing" +
	"?start=1&limit=%d&sortBy=volume_24h&sortType=desc&convert=USD&cryptoType=all&tagType=all"

// pageURL is the page the user asked for, used as the fallback source.
const pageURL = "https://coinmarketcap.com/all/views/all/"

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Scraper crawls CoinMarketCap for the most liquid assets.
type Scraper struct {
	TopN    int
	Timeout time.Duration
}

func (s *Scraper) collector(domains ...string) *colly.Collector {
	c := colly.NewCollector(
		colly.AllowedDomains(domains...),
		colly.UserAgent(userAgent),
	)
	c.SetRequestTimeout(s.Timeout)
	extensions.Referer(c)
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		log.Printf("crawler: GET %s", r.URL)
	})
	return c
}

// Scrape performs one crawl and returns the volume-ranked, stablecoin-free set.
func (s *Scraper) Scrape() Snapshot {
	start := time.Now()

	snap, err := s.scrapeListing()
	if err != nil {
		log.Printf("crawler: listing source failed (%v); falling back to HTML table", err)
		var htmlErr error
		snap, htmlErr = s.scrapeHTML()
		if htmlErr != nil {
			snap.Error = fmt.Sprintf("listing: %v; html fallback: %v", err, htmlErr)
		}
	}

	sort.SliceStable(snap.Assets, func(i, j int) bool {
		return snap.Assets[i].Volume24h > snap.Assets[j].Volume24h
	})
	if s.TopN > 0 && len(snap.Assets) > s.TopN {
		snap.Assets = snap.Assets[:s.TopN]
	}
	for i := range snap.Assets {
		snap.Assets[i].Rank = i + 1
		snap.TotalVolume += snap.Assets[i].Volume24h
	}

	snap.FetchedAt = start
	snap.DurationMS = time.Since(start).Milliseconds()
	if snap.Error == "" && len(snap.Assets) == 0 {
		snap.Error = "crawl returned no assets — CoinMarketCap may have changed its markup"
	}
	log.Printf("crawler: source=%s rows=%d excluded(stablecoin=%d derivative=%d tokenized=%d) kept=%d (%dms)",
		snap.Source, snap.RowsSeen, snap.StablesFound, snap.DerivsFound, snap.TokenizedHit,
		len(snap.Assets), snap.DurationMS)
	return snap
}

// --- primary source: JSON listing -------------------------------------------

type cmcListing struct {
	Data struct {
		List []struct {
			Name              string   `json:"name"`
			Symbol            string   `json:"symbol"`
			Slug              string   `json:"slug"`
			CirculatingSupply float64  `json:"circulatingSupply"`
			Tags              []string `json:"tags"`
			Quotes            []struct {
				Price           float64 `json:"price"`
				MarketCap       float64 `json:"marketCap"`
				Volume24h       float64 `json:"volume24h"`
				PercentChange1h float64 `json:"percentChange1h"`
				PercentChange24 float64 `json:"percentChange24h"`
				PercentChange7d float64 `json:"percentChange7d"`
			} `json:"quotes"`
		} `json:"cryptoCurrencyList"`
	} `json:"data"`
	Status struct {
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"status"`
}

func (s *Scraper) scrapeListing() (Snapshot, error) {
	snap := Snapshot{Source: "coinmarketcap listing api"}

	// Ask for a generous window so we still have plenty left after filtering.
	limit := s.TopN * 2
	if limit < 200 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	c := s.collector("api.coinmarketcap.com")
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "application/json")
	})

	var parseErr error
	c.OnResponse(func(r *colly.Response) {
		var out cmcListing
		if err := json.Unmarshal(r.Body, &out); err != nil {
			parseErr = fmt.Errorf("decode listing json: %w", err)
			return
		}
		if out.Status.ErrorMessage != "" && out.Status.ErrorMessage != "SUCCESS" {
			parseErr = fmt.Errorf("listing api: %s", out.Status.ErrorMessage)
			return
		}

		for _, it := range out.Data.List {
			if len(it.Quotes) == 0 {
				continue
			}
			q := it.Quotes[0]
			snap.RowsSeen++

			a := Asset{
				Name:              it.Name,
				Symbol:            it.Symbol,
				URL:               "https://coinmarketcap.com/currencies/" + it.Slug + "/",
				Price:             q.Price,
				PriceText:         formatPrice(q.Price),
				MarketCap:         q.MarketCap,
				Volume24h:         q.Volume24h,
				CirculatingSupply: formatSupply(it.CirculatingSupply, it.Symbol),
				Change1h:          q.PercentChange1h,
				Change24h:         q.PercentChange24,
				Change7d:          q.PercentChange7d,
			}

			if hasStablecoinTag(it.Tags) || isStablecoin(a) {
				snap.StablesFound++
				continue
			}
			if isDerivative(a, it.Tags) {
				snap.DerivsFound++
				continue
			}
			if isTokenizedAsset(a, it.Tags) {
				snap.TokenizedHit++
				continue
			}
			snap.Assets = append(snap.Assets, a)
		}
	})

	var reqErr error
	c.OnError(func(r *colly.Response, err error) {
		reqErr = fmt.Errorf("status %d: %w", r.StatusCode, err)
	})

	if err := c.Visit(fmt.Sprintf(listingURL, limit)); err != nil {
		return snap, err
	}
	c.Wait()

	switch {
	case reqErr != nil:
		return snap, reqErr
	case parseErr != nil:
		return snap, parseErr
	case len(snap.Assets) == 0:
		return snap, fmt.Errorf("listing returned no usable rows")
	}
	return snap, nil
}

// hasStablecoinTag uses CoinMarketCap's own classification, which covers
// fiat-, asset-backed- and algorithmic-stablecoin variants.
func hasStablecoinTag(tags []string) bool {
	for _, t := range tags {
		if strings.Contains(strings.ToLower(t), "stablecoin") {
			return true
		}
	}
	return false
}

// --- fallback source: the rendered HTML table --------------------------------

// scrapeHTML parses the /all/views/all/ table. Only the rows CoinMarketCap
// server-renders (currently the top 20 by market cap) carry real data; the
// remaining rows are client-side skeletons and are skipped.
func (s *Scraper) scrapeHTML() (Snapshot, error) {
	snap := Snapshot{Source: "coinmarketcap html table (fallback)"}

	c := s.collector("coinmarketcap.com", "www.coinmarketcap.com")
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	})

	c.OnHTML("tbody tr.cmc-table-row", func(e *colly.HTMLElement) {
		name := strings.TrimSpace(e.ChildText("a.cmc-table__column-name--name"))
		symbol := strings.TrimSpace(e.ChildText("a.cmc-table__column-name--symbol"))
		if name == "" || symbol == "" {
			return // unhydrated skeleton row
		}
		snap.RowsSeen++

		a := Asset{
			Name:              name,
			Symbol:            symbol,
			CirculatingSupply: cell(e, "circulating-supply"),
			PriceText:         cell(e, "price"),
			MarketCapText:     cell(e, "market-cap"),
			Volume24hText:     cell(e, "volume-24-h"),
		}
		if href := e.ChildAttr("a.cmc-table__column-name--name", "href"); href != "" {
			a.URL = e.Request.AbsoluteURL(href)
		}
		a.Price = parseMoney(a.PriceText)
		a.MarketCap = parseMoney(a.MarketCapText)
		a.Volume24h = parseMoney(a.Volume24hText)
		a.Change1h = parsePercent(cell(e, "percent-change-1-h"))
		a.Change24h = parsePercent(cell(e, "percent-change-24-h"))
		a.Change7d = parsePercent(cell(e, "percent-change-7-d"))

		if isStablecoin(a) {
			snap.StablesFound++
			return
		}
		// No tags on the HTML path — name detection carries it.
		if isDerivative(a, nil) {
			snap.DerivsFound++
			return
		}
		if isTokenizedAsset(a, nil) {
			snap.TokenizedHit++
			return
		}
		snap.Assets = append(snap.Assets, a)
	})

	var reqErr error
	c.OnError(func(r *colly.Response, err error) {
		reqErr = fmt.Errorf("status %d: %w", r.StatusCode, err)
	})

	if err := c.Visit(pageURL); err != nil {
		return snap, err
	}
	c.Wait()
	if reqErr != nil {
		return snap, reqErr
	}
	if len(snap.Assets) == 0 {
		return snap, fmt.Errorf("no hydrated rows found in HTML table")
	}
	return snap, nil
}

// cell reads a table cell by CoinMarketCap's stable `sort-by__<field>` class.
func cell(e *colly.HTMLElement, field string) string {
	return strings.TrimSpace(e.ChildText("td.cmc-table__cell--sort-by__" + field))
}

// --- formatting / parsing helpers -------------------------------------------

func formatPrice(v float64) string {
	switch {
	case v == 0:
		return "—"
	case v >= 1:
		return "$" + addThousands(strconv.FormatFloat(v, 'f', 2, 64))
	case v >= 0.01:
		return "$" + strconv.FormatFloat(v, 'f', 4, 64)
	default:
		return "$" + strconv.FormatFloat(v, 'f', 8, 64)
	}
}

func formatSupply(v float64, symbol string) string {
	if v <= 0 {
		return "—"
	}
	return addThousands(strconv.FormatFloat(v, 'f', 0, 64)) + " " + symbol
}

// addThousands inserts commas into the integer part of a plain decimal string.
func addThousands(s string) string {
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	neg := strings.HasPrefix(intPart, "-")
	intPart = strings.TrimPrefix(intPart, "-")

	var b strings.Builder
	for i, d := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	out := b.String() + frac
	if neg {
		out = "-" + out
	}
	return out
}

// parseMoney turns "$1.54T", "$50,764,798,921" or "$0.00001234" into a float.
func parseMoney(s string) float64 {
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(strings.TrimSpace(s))
	if s == "" || s == "--" || s == "?" {
		return 0
	}
	mult := 1.0
	switch s[len(s)-1] {
	case 'T':
		mult, s = 1e12, s[:len(s)-1]
	case 'B':
		mult, s = 1e9, s[:len(s)-1]
	case 'M':
		mult, s = 1e6, s[:len(s)-1]
	case 'K':
		mult, s = 1e3, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v * mult
}

// parsePercent turns "-3.57%" into -3.57.
func parsePercent(s string) float64 {
	s = strings.NewReplacer("%", "", ",", "", " ", "").Replace(strings.TrimSpace(s))
	if s == "" || s == "--" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
