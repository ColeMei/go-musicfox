package track

import (
	"errors"
	"net/url"
	"sync"
	"time"
)

// cdnAlternates maps NetEase audio CDN hosts to a sibling host that serves the
// same signed paths (both CNAME to the same edge network).
var cdnAlternates = map[string]string{
	"m801.music.126.net": "m701.music.126.net",
	"m701.music.126.net": "m801.music.126.net",
}

// cdnBadHostTTL is how long a host stays marked unreachable, so a changed
// network (home vs. campus) is picked up again.
const cdnBadHostTTL = 30 * time.Minute

// StreamFailure describes a remote stream that could not be opened.
type StreamFailure struct {
	SongID int64
	Host   string
	// HostMarkedBad is set on the failure that first marked Host unreachable.
	HostMarkedBad bool
	Err           error
}

// cdnFallback remembers CDN hosts whose streams fail at the connection level
// (for example a host sinkholed by the local network's DNS) and rewrites new
// song URLs on such a host to its alternate.
type cdnFallback struct {
	mu  sync.Mutex
	bad map[string]time.Time
	now func() time.Time
}

func (c *cdnFallback) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *cdnFallback) isBadLocked(host string) bool {
	at, ok := c.bad[host]
	if ok && c.clock().Sub(at) > cdnBadHostTTL {
		delete(c.bad, host)
		return false
	}
	return ok
}

// markBad records host of rawURL as unreachable when err is a transport error
// (connection reset, refused, DNS, timeout) rather than an HTTP status.
func (c *cdnFallback) markBad(rawURL string, err error) (host string, first bool) {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return "", false
	}
	u, parseErr := url.Parse(rawURL)
	if parseErr != nil || u.Host == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	first = !c.isBadLocked(u.Host)
	if c.bad == nil {
		c.bad = make(map[string]time.Time)
	}
	c.bad[u.Host] = c.clock()
	return u.Host, first
}

// rewrite swaps a known-bad host for its alternate, unless that is bad too.
func (c *cdnFallback) rewrite(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, false
	}
	alt, ok := cdnAlternates[u.Host]
	if !ok {
		return rawURL, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isBadLocked(u.Host) || c.isBadLocked(alt) {
		return rawURL, false
	}
	u.Host = alt
	return u.String(), true
}
