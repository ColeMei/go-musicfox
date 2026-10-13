package ui

import (
	"testing"
	"time"
)

// armNoFire arms g with a timer that never fires during the test, so check is
// driven explicitly with the returned generation.
func armNoFire(g *stallGuard, songID int64) uint64 {
	g.arm(songID, true, time.Hour, func(uint64) {})
	gen, _ := g.pending(songID)
	return gen
}

func TestStallGuardRetriesThenSkipsThenGivesUp(t *testing.T) {
	var g stallGuard
	var got []stallAction
	for song := int64(1); song <= stallMaxSkips; song++ {
		for {
			action := g.check(armNoFire(&g, song), false, false)
			got = append(got, action)
			if action != stallRetry {
				break
			}
		}
	}

	var want []stallAction
	for song := 1; song <= stallMaxSkips; song++ {
		for range stallMaxRetries {
			want = append(want, stallRetry)
		}
		if song < stallMaxSkips {
			want = append(want, stallSkip)
		} else {
			want = append(want, stallGiveUp)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d actions %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("action %d: got %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestStallGuardStartedResetsCounters(t *testing.T) {
	var g stallGuard
	for range stallMaxRetries {
		g.check(armNoFire(&g, 1), false, false)
	}
	if action := g.check(armNoFire(&g, 1), true, false); action != stallNone {
		t.Fatalf("started: got %v, want none", action)
	}
	if action := g.check(armNoFire(&g, 1), false, false); action != stallRetry {
		t.Fatalf("after start, a new stall should retry from scratch, got %v", action)
	}
}

func TestStallGuardSongChangeResetsTries(t *testing.T) {
	var g stallGuard
	for range stallMaxRetries {
		g.check(armNoFire(&g, 1), false, false)
	}
	if action := g.check(armNoFire(&g, 2), false, false); action != stallRetry {
		t.Fatalf("new song should get fresh retries, got %v", action)
	}
}

func TestStallGuardIgnoresStaleGeneration(t *testing.T) {
	var g stallGuard
	stale := armNoFire(&g, 1)
	armNoFire(&g, 2)
	if action := g.check(stale, false, false); action != stallNone {
		t.Fatalf("stale generation acted: %v", action)
	}
	if _, ok := g.pending(1); ok {
		t.Fatal("song 1 should no longer be pending")
	}
	if _, ok := g.pending(2); !ok {
		t.Fatal("song 2 should be pending")
	}
}

func TestStallGuardCheckConsumesGeneration(t *testing.T) {
	var g stallGuard
	gen := armNoFire(&g, 1)
	if action := g.check(gen, false, false); action != stallRetry {
		t.Fatalf("got %v, want retry", action)
	}
	// Timer and fast path racing on the same generation must act only once.
	if action := g.check(gen, false, false); action != stallNone {
		t.Fatalf("second check of same generation acted: %v", action)
	}
}

func TestStallGuardPausedKeepsWatching(t *testing.T) {
	var g stallGuard
	gen := armNoFire(&g, 1)
	if action := g.check(gen, false, true); action != stallNone {
		t.Fatalf("paused: got %v, want none", action)
	}
	if _, ok := g.pending(1); !ok {
		t.Fatal("paused song should stay pending")
	}
	if action := g.check(gen, false, false); action != stallRetry {
		t.Fatalf("after resume still stalled: got %v, want retry", action)
	}
}

func TestStallGuardLocalSourceNotWatched(t *testing.T) {
	var g stallGuard
	g.arm(1, false, time.Hour, func(uint64) {})
	if _, ok := g.pending(1); ok {
		t.Fatal("local source should not be watched")
	}
}

func TestStallGuardTimerFires(t *testing.T) {
	var g stallGuard
	fired := make(chan uint64, 1)
	g.arm(1, true, 10*time.Millisecond, func(gen uint64) { fired <- gen })
	select {
	case gen := <-fired:
		if action := g.check(gen, false, false); action != stallRetry {
			t.Fatalf("got %v, want retry", action)
		}
	case <-time.After(time.Second):
		t.Fatal("timer did not fire")
	}
}

func TestIsRemotePlayURL(t *testing.T) {
	for url, want := range map[string]bool{
		"http://m801.music.126.net/a.flac": true,
		"https://example.com/a.mp3":        true,
		"file:///tmp/a.flac":               false,
		"":                                 false,
	} {
		if got := isRemotePlayURL(url); got != want {
			t.Errorf("isRemotePlayURL(%q) = %v, want %v", url, got, want)
		}
	}
}
