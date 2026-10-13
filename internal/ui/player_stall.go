package ui

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/anhoder/foxful-cli/model"

	"github.com/go-musicfox/go-musicfox/internal/player"
	"github.com/go-musicfox/go-musicfox/internal/track"
	"github.com/go-musicfox/go-musicfox/internal/types"
	"github.com/go-musicfox/go-musicfox/utils/notify"
)

const (
	// stallTimeout is how long a remote song may stay at 0:00 before it is
	// treated as stalled and replayed with a freshly resolved URL.
	stallTimeout = 6 * time.Second
	// stallMaxRetries bounds replays of one song. Each replay re-requests the
	// URL, which NetEase often serves from a different CDN host.
	stallMaxRetries = 5
	// stallMaxSkips bounds consecutive songs skipped for stalling before the
	// player gives up, so a dead network does not walk the whole playlist.
	stallMaxSkips = 3
)

type stallAction int

const (
	stallNone stallAction = iota
	stallRetry
	stallSkip
	stallGiveUp
)

// stallGuard detects remote songs whose playback never starts (for example a
// CDN host that the local network resets or sinkholes) and decides whether to
// replay, skip or give up. Every arm bumps gen so stale timers and stale
// stream-failure reports are ignored.
type stallGuard struct {
	mu      sync.Mutex
	gen     uint64
	songID  int64
	tries   int
	skips   int
	timer   *time.Timer
	timeout time.Duration
	fire    func(gen uint64)
}

// arm starts watching songID. Tries are kept when the same song is re-armed
// (a replay) and reset when the song changes. Local sources are not watched.
func (g *stallGuard) arm(songID int64, remote bool, timeout time.Duration, fire func(gen uint64)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gen++
	g.stopTimerLocked()
	if songID != g.songID {
		g.songID = songID
		g.tries = 0
	}
	if !remote {
		return
	}
	gen := g.gen
	g.timeout, g.fire = timeout, fire
	g.timer = time.AfterFunc(timeout, func() { fire(gen) })
}

// disarm stops watching without changing counters.
func (g *stallGuard) disarm() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gen++
	g.stopTimerLocked()
}

// pending reports the generation currently watching songID, if any.
func (g *stallGuard) pending(songID int64) (uint64, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.timer == nil || g.songID != songID {
		return 0, false
	}
	return g.gen, true
}

// check evaluates generation gen and returns the action plus, for a retry,
// its 1-based attempt number. A paused song is re-checked later without
// consuming a try; a started song clears all counters.
func (g *stallGuard) check(gen uint64, started, paused bool) (stallAction, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if gen != g.gen || g.timer == nil {
		return stallNone, 0
	}
	if paused {
		g.stopTimerLocked()
		fire, timeout := g.fire, g.timeout
		g.timer = time.AfterFunc(timeout, func() { fire(gen) })
		return stallNone, 0
	}
	g.gen++
	g.stopTimerLocked()
	if started {
		g.tries, g.skips = 0, 0
		return stallNone, 0
	}
	g.tries++
	if g.tries <= stallMaxRetries {
		return stallRetry, g.tries
	}
	g.tries = 0
	g.skips++
	if g.skips >= stallMaxSkips {
		g.skips = 0
		return stallGiveUp, 0
	}
	return stallSkip, 0
}

func (g *stallGuard) stopTimerLocked() {
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
}

func isRemotePlayURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")
}

// watchStartup arms the stall guard for a song that was just handed to the
// engine.
func (p *Player) watchStartup(songID int64, url string) {
	p.stall.arm(songID, isRemotePlayURL(url), stallTimeout, p.checkStall)
}

// handleStreamFailure is the fast path: the background cache download hits the
// same CDN as the engine, so its failure while the song is still at 0:00 means
// the engine is almost certainly stuck too.
func (p *Player) handleStreamFailure(f track.StreamFailure) {
	if f.HostMarkedBad {
		notify.Toast(notify.NotifyContent{
			Title:   "CDN 节点无法连接",
			Text:    f.Host + "，后续歌曲将自动改用备用节点",
			Level:   notify.ToastWarning,
			Timeout: 6 * time.Second,
		})
	}
	gen, ok := p.stall.pending(f.SongID)
	if !ok || p.PassedTime() > 0 {
		return
	}
	slog.Warn("Stream failed before playback started, replaying", "songId", f.SongID, "error", f.Err)
	p.checkStall(gen)
}

func (p *Player) checkStall(gen uint64) {
	state := p.State()
	// A stopped song already left startup (ended or stopped by the user), so
	// there is nothing to recover; treat it like a started one.
	started := p.PassedTime() > 0 || state == types.Stopped
	action, attempt := p.stall.check(gen, started, state == types.Paused)
	song := p.CurSong()
	switch action {
	case stallRetry:
		slog.Warn("Playback stalled, replaying with a fresh URL", "songId", song.Id, "song", song.Name, "attempt", attempt)
		notify.Toast(notify.NotifyContent{
			Title:   fmt.Sprintf("加载卡住，正在重试 (%d/%d)", attempt, stallMaxRetries),
			Text:    song.Name,
			Level:   notify.ToastInfo,
			Timeout: 3 * time.Second,
		})
		p.replayStalled()
	case stallSkip:
		slog.Warn("Playback stalled repeatedly, skipping song", "songId", song.Id, "song", song.Name)
		notify.Toast(notify.NotifyContent{
			Title:   "无法加载，已跳过",
			Text:    song.Name,
			Level:   notify.ToastWarning,
			Timeout: 5 * time.Second,
		})
		p.NextSong(false)
	case stallGiveUp:
		slog.Error("Playback stalled on several songs in a row, stopping", "songId", song.Id)
		p.Pause()
		notify.Notify(notify.NotifyContent{
			Title:   "播放失败",
			Text:    "连续多首歌曲无法加载，请检查网络",
			GroupId: types.GroupID,
			Level:   notify.ToastWarning,
		})
	}
}

// replayStalled re-resolves the current song and hands the new URL to the
// engine, without the side effects of PlaySong (reporting, notifications).
func (p *Player) replayStalled() {
	song := p.CurSong()
	loading := model.NewLoading(p.netease.MustMain())
	loading.Start()
	defer loading.Complete()

	url, musicType, err := p.getPlayInfo(song)
	if url == "" || err != nil {
		slog.Error("Replay stalled song: resolve failed, skipping", "songId", song.Id, slog.Any("err", err))
		p.NextSong(false)
		return
	}
	if p.CurSong().Id != song.Id {
		return
	}
	p.Play(player.URLMusic{
		URL:  url,
		Song: song,
		Type: player.SongTypeMapping[musicType],
	})
	slog.Info("Replay stalled song", "url", url, "type", musicType, "songId", song.Id)
	p.watchStartup(song.Id, url)
}
