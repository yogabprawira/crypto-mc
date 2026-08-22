package main

import (
	"log"
	"sync"
	"time"
)

// Store holds the latest snapshot and guarantees that only one crawl runs at a
// time, so the manual refresh button and the scheduler cannot stampede the
// source. A refresh requested while one is in flight joins the running crawl
// instead of starting a second one.
type Store struct {
	scraper *Scraper

	mu      sync.RWMutex
	current Snapshot

	runMu   sync.Mutex
	running bool
	done    chan struct{}
}

func NewStore(s *Scraper) *Store {
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

	// Keep the previous good data visible if the crawl failed outright.
	st.mu.Lock()
	if snap.Error != "" && len(snap.Assets) == 0 && len(st.current.Assets) > 0 {
		prev := st.current
		prev.Error = snap.Error
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

// StartScheduler kicks off periodic refreshes until stop is closed.
func (st *Store) StartScheduler(interval time.Duration, stop <-chan struct{}) {
	if interval <= 0 {
		log.Printf("scheduler: disabled (SCRAPE_INTERVAL <= 0)")
		return
	}
	log.Printf("scheduler: running every %s", interval)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				st.Refresh("scheduler")
			case <-stop:
				log.Printf("scheduler: stopped")
				return
			}
		}
	}()
}
