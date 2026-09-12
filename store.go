package main

import (
	"log"
	"sync"
	"time"
)

// crawler is the scraping dependency; *Scraper satisfies it.
type crawler interface {
	Scrape() Snapshot
}

// Store holds the latest snapshot and guarantees that only one crawl runs at a
// time, so the manual refresh button and the scheduler cannot stampede the
// source. A refresh requested while one is in flight joins the running crawl
// instead of starting a second one.
type Store struct {
	scraper crawler

	mu      sync.RWMutex
	current Snapshot

	runMu   sync.Mutex
	running bool
	done    chan struct{}
}

func NewStore(s crawler) *Store {
	return &Store{scraper: s}
}

// Snapshot returns the last successful crawl result.
func (st *Store) Snapshot() Snapshot {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.current
}

// Refresh crawls and stores a new snapshot. It reports whether this call did
// the actual crawling (false means it waited on an in-flight one).
func (st *Store) Refresh(trigger string) (Snapshot, bool) {
	st.runMu.Lock()
	if st.running {
		wait := st.done
		st.runMu.Unlock()
		<-wait
		return st.Snapshot(), false
	}
	st.running = true
	st.done = make(chan struct{})
	done := st.done
	st.runMu.Unlock()

	log.Printf("refresh: started (trigger=%s)", trigger)
	snap := st.scraper.Scrape()

	// Keep the previous good data visible if the crawl failed outright, but
	// stamp the failed attempt's time/duration/error on it so the UI reflects
	// that a crawl did run.
	st.mu.Lock()
	if snap.Error != "" && len(snap.Assets) == 0 && len(st.current.Assets) > 0 {
		prev := st.current
		prev.Error = snap.Error
		prev.FetchedAt = snap.FetchedAt
		prev.DurationMS = snap.DurationMS
		st.current = prev
	} else {
		st.current = snap
	}
	result := st.current
	st.mu.Unlock()

	st.runMu.Lock()
	st.running = false
	close(done)
	st.runMu.Unlock()

	return result, true
}

// Trigger starts a crawl in the background without blocking the caller. If a
// crawl is already in flight the request joins it; Store's single-flight
// Refresh prevents any source stampede either way.
func (st *Store) Trigger(trigger string) {
	st.runMu.Lock()
	if st.running {
		st.runMu.Unlock()
		log.Printf("refresh: %s joined an in-flight crawl", trigger)
		return
	}
	st.runMu.Unlock()
	go st.Refresh(trigger)
}

// StartScheduler kicks off periodic refreshes until stop is closed.
func (st *Store) StartScheduler(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		log.Printf("scheduler: disabled (SCRAPE_INTERVAL <= 0)")
		return
	}
	log.Printf("scheduler: running every %s", interval)

	go func() {
		// time.After is re-created after each Refresh completes, so the next
		// crawl always starts a full interval after the previous one — no
		// buffered-tick back-to-back crawls when interval < crawl duration.
		for {
			select {
			case <-time.After(interval):
				st.Refresh("scheduler")
			case <-stop:
				log.Printf("scheduler: stopped")
				return
			}
		}
	}()
}
