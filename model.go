package main

import "time"

// Asset is a single cryptocurrency row scraped from CoinMarketCap.
type Asset struct {
	Rank              int     `json:"rank"`
	Name              string  `json:"name"`
	Symbol            string  `json:"symbol"`
	URL               string  `json:"url"`
	Price             float64 `json:"price"`
	PriceText         string  `json:"priceText"`
	MarketCap         float64 `json:"marketCap"`
	MarketCapText     string  `json:"marketCapText"`
	Volume24h         float64 `json:"volume24h"`
	Volume24hText     string  `json:"volume24hText"`
	CirculatingSupply string  `json:"circulatingSupply"`
	Change1h          float64 `json:"change1h"`
	Change24h         float64 `json:"change24h"`
	Change7d          float64 `json:"change7d"`
}

// Snapshot is the result of one crawl run, served to the frontend as-is.
type Snapshot struct {
	Assets       []Asset   `json:"assets"`
	Source       string    `json:"source"`
	FetchedAt    time.Time `json:"fetchedAt"`
	DurationMS   int64     `json:"durationMs"`
	RowsSeen     int       `json:"rowsSeen"`
	StablesFound int       `json:"stablesExcluded"`
	DerivsFound  int       `json:"derivativesExcluded"`
	TokenizedHit int       `json:"tokenizedExcluded"`
	TotalVolume  float64   `json:"totalVolume"`
	Error        string    `json:"error,omitempty"`
}
