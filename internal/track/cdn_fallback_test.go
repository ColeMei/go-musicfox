package track

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"testing"
	"time"

	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/utils/netease"
)

const m801URL = "http://m801.music.126.net/20261013105547/abc/jdymusic/obj/x.flac?vuutv=1"

func connReset(rawURL string) error {
	return &url.Error{Op: "Get", URL: rawURL, Err: errors.New("read: connection reset by peer")}
}

func TestCDNFallbackRewritesOnlyAfterTransportFailure(t *testing.T) {
	var c cdnFallback
	if _, ok := c.rewrite(m801URL); ok {
		t.Fatal("healthy host must not be rewritten")
	}
	if host, _ := c.markBad(m801URL, fmt.Errorf("invalid http status: 403")); host != "" {
		t.Fatal("HTTP status errors must not mark a host bad")
	}

	host, first := c.markBad(m801URL, fmt.Errorf("http get failed: %w", connReset(m801URL)))
	if host != "m801.music.126.net" || !first {
		t.Fatalf("markBad = (%q, %v), want (m801.music.126.net, true)", host, first)
	}
	if _, first := c.markBad(m801URL, connReset(m801URL)); first {
		t.Fatal("second failure must not be reported as first")
	}

	got, ok := c.rewrite(m801URL)
	want := "http://m701.music.126.net/20261013105547/abc/jdymusic/obj/x.flac?vuutv=1"
	if !ok || got != want {
		t.Fatalf("rewrite = (%q, %v), want (%q, true)", got, ok, want)
	}
}

func TestCDNFallbackKeepsURLWhenAlternateAlsoBad(t *testing.T) {
	var c cdnFallback
	m701URL := "http://m701.music.126.net/a.flac"
	c.markBad(m801URL, connReset(m801URL))
	c.markBad(m701URL, connReset(m701URL))
	if _, ok := c.rewrite(m801URL); ok {
		t.Fatal("must not rewrite onto a host that is also bad")
	}
}

func TestCDNFallbackExpires(t *testing.T) {
	now := time.Unix(0, 0)
	c := cdnFallback{now: func() time.Time { return now }}
	c.markBad(m801URL, connReset(m801URL))
	now = now.Add(cdnBadHostTTL + time.Second)
	if _, ok := c.rewrite(m801URL); ok {
		t.Fatal("bad host must expire after TTL")
	}
}

func TestCDNFallbackIgnoresUnknownHosts(t *testing.T) {
	var c cdnFallback
	other := "http://example.com/a.flac"
	c.markBad(other, connReset(other))
	if _, ok := c.rewrite(other); ok {
		t.Fatal("hosts without an alternate must not be rewritten")
	}
}

type countingFailFetcher struct {
	lyricFetcherStub
	calls int
}

func (f *countingFailFetcher) FetchStream(_ context.Context, source PlayableSource) (io.ReadCloser, error) {
	f.calls++
	return nil, connReset(source.Info.URL)
}

// A handler that replays the song (re-entering backgroundCache) must trigger a
// real fetch, not be folded into the failing call by singleflight.
func TestBackgroundCacheHandlerCanReenter(t *testing.T) {
	fetcher := &countingFailFetcher{}
	m := &Manager{fetcher: fetcher}
	source := PlayableSource{
		Song: structs.Song{Id: 7},
		Type: SourceRemote,
		Info: &netease.PlayableInfo{URL: m801URL, MusicType: "flac"},
	}
	var failures []StreamFailure
	m.SetStreamFailureHandler(func(f StreamFailure) {
		failures = append(failures, f)
		if len(failures) == 1 {
			m.backgroundCache(context.Background(), source)
		}
	})

	m.backgroundCache(context.Background(), source)

	if fetcher.calls != 2 {
		t.Fatalf("fetch calls = %d, want 2", fetcher.calls)
	}
	if len(failures) != 2 || !failures[0].HostMarkedBad || failures[1].HostMarkedBad {
		t.Fatalf("failures = %+v, want first marked bad, second not", failures)
	}
}
