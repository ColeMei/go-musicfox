package track

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-musicfox/go-musicfox/internal/structs"
	"github.com/go-musicfox/go-musicfox/utils/netease"
)

type failingStreamFetcher struct {
	lyricFetcherStub
	err error
}

func (f *failingStreamFetcher) FetchStream(context.Context, PlayableSource) (io.ReadCloser, error) {
	return nil, f.err
}

func TestBackgroundCacheReportsStreamFailure(t *testing.T) {
	wantErr := errors.New("read: connection reset by peer")
	m := &Manager{fetcher: &failingStreamFetcher{err: wantErr}}

	var gotID int64
	var gotErr error
	m.SetStreamFailureHandler(func(songID int64, err error) {
		gotID, gotErr = songID, err
	})

	m.backgroundCache(context.Background(), PlayableSource{
		Song: structs.Song{Id: 42},
		Type: SourceRemote,
		Info: &netease.PlayableInfo{URL: "http://m801.music.126.net/x.flac", MusicType: "flac"},
	})

	if gotID != 42 || !errors.Is(gotErr, wantErr) {
		t.Fatalf("handler got (%d, %v), want (42, %v)", gotID, gotErr, wantErr)
	}
}
