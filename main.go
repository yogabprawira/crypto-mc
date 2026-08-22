package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

//go:embed web/index.html
var webFS embed.FS

type Config struct {
	Port          string
	Interval      time.Duration
	TopN          int
	Timeout       time.Duration
	ScrapeOnStart bool
}

func loadConfig() Config {
	return Config{
		Port:          envStr("PORT", "8080"),
		Interval:      envDur("SCRAPE_INTERVAL", 15*time.Minute),
		TopN:          envInt("TOP_N", 200),
		Timeout:       envDur("SCRAPE_TIMEOUT", 45*time.Second),
		ScrapeOnStart: envBool("SCRAPE_ON_START", true),
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("crypto-mc ")

	cfg := loadConfig()
	log.Printf("config: port=%s interval=%s topN=%d timeout=%s scrapeOnStart=%t",
		cfg.Port, cfg.Interval, cfg.TopN, cfg.Timeout, cfg.ScrapeOnStart)

	store := NewStore(&Scraper{TopN: cfg.TopN, Timeout: cfg.Timeout})

	stop := make(chan struct{})
	if cfg.ScrapeOnStart {
		go store.Refresh("startup")
	}
	store.StartScheduler(cfg.Interval, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/assets", handleAssets(store))
	mux.HandleFunc("/api/refresh", handleRefresh(store))
	mux.HandleFunc("/api/config", handleConfig(cfg))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      2 * time.Minute,
	}

	go func() {
		log.Printf("server: listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Printf("server: shutting down")
	close(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	page, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page)
}

func handleAssets(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, store.Snapshot())
	}
}

// handleRefresh triggers the crawler on demand (the "Crawl again" button).
func handleRefresh(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		snap, didWork := store.Refresh("manual")
		if !didWork {
			log.Printf("refresh: manual request joined an in-flight crawl")
		}
		writeJSON(w, http.StatusOK, snap)
	}
}

func handleConfig(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"intervalSeconds": int(cfg.Interval.Seconds()),
			"interval":        cfg.Interval.String(),
			"topN":            cfg.TopN,
			"source":          pageURL,
		})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("config: %s=%q is not an integer, using %d", key, v, def)
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
		log.Printf("config: %s=%q is not a bool, using %t", key, v, def)
	}
	return def
}

// envDur accepts Go durations ("30s", "15m", "1h30m") and bare seconds ("900").
func envDur(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	log.Printf("config: %s=%q is not a duration, using %s", key, v, def)
	return def
}
