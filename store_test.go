package main

import (
	"sync"
	"testing"
	"time"
)

// fakeScraper is a controllable crawler for Store tests.
type fakeScraper struct {
	mu    sync.Mutex
	snaps  []Snapshot // returned in order; last one repeats
	calls  int
	starts []time.Time
	block  time.Duration
}

func (f *fakeScraper) Scrape() Snapshot {
	f.mu.Lock()
	f.calls++
	f.starts = append(f.starts, time.Now())
	snap := f.snaps[min(f.calls-1, len(f.snaps)-1)]
	f.mu.Unlock()
	if f.block > 0 {
		time.Sleep(f.block)
	}
	return snap
}

// A crawl that fails outright must keep the previous good assets but carry
// the failed attempt's fresh FetchedAt/DurationMS/Error.
func TestRefreshErrorKeepsPrevDataWithFreshTimestamp(t *testing.T) {
	t1 := time.Now().Add(-time.Hour)
	good := Snapshot{Assets: []Asset{{Symbol: "BTC"}}, FetchedAt: t1}
	t2 := time.Now()
	bad := Snapshot{Error: "boom", FetchedAt: t2, DurationMS: 123}

	st := NewStore(&fakeScraper{snaps: []Snapshot{good, bad}})
	st.Refresh("first")
	st.Refresh("second")

	got := st.Snapshot()
	if len(got.Assets) != 1 || got.Assets[0].Symbol != "BTC" {
		t.Fatalf("previous assets lost: %+v", got.Assets)
	}
	if got.Error != "boom" {
		t.Fatalf("error not surfaced: %q", got.Error)
	}
	if !got.FetchedAt.Equal(t2) {
		t.Fatalf("FetchedAt = %v, want %v", got.FetchedAt, t2)
	}
	if got.DurationMS != 123 {
		t.Fatalf("DurationMS = %d, want 123", got.DurationMS)
	}
}

// Trigger must return immediately and run the crawl in the background.
func TestTriggerDoesNotBlock(t *testing.T) {
	st := NewStore(&fakeScraper{
		snaps: []Snapshot{{Assets: []Asset{{Symbol: "X"}}}},
		block: 100 * time.Millisecond,
	})

	start := time.Now()
	st.Trigger("manual")
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("Trigger blocked for %v, want immediate return", elapsed)
	}

	deadline := time.Now().Add(3 * time.Second)
	for len(st.Snapshot().Assets) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(st.Snapshot().Assets) != 1 {
		t.Fatal("background crawl never completed")
	}
}

// The scheduler must wait a full interval after a crawl finishes, not fire a
// buffered tick immediately (back-to-back crawls when interval < crawl time).
// Crawl takes 150ms, interval is 100ms, so consecutive crawl starts must be
// at least ~250ms apart, not ~150ms.
func TestSchedulerWaitsIntervalAfterCrawl(t *testing.T) {
	f := &fakeScraper{snaps: []Snapshot{{}}, block: 150 * time.Millisecond}
	st := NewStore(f)

	stop := make(chan struct{})
	st.StartScheduler(100*time.Millisecond, stop)
	time.Sleep(750 * time.Millisecond)
	close(stop)

	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.starts) < 3 {
		t.Fatalf("only %d crawls ran, want at least 3", len(f.starts))
	}
	for i := 1; i < len(f.starts); i++ {
		gap := f.starts[i].Sub(f.starts[i-1])
		if gap < 230*time.Millisecond {
			t.Fatalf("crawls %d and %d back-to-back: gap %v, want >= 230ms", i-1, i, gap)
		}
	}
}
