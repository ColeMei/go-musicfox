package ui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
)

func textEntries(texts ...string) []lyricEntry {
	entries := make([]lyricEntry, len(texts))
	for i, text := range texts {
		entries[i] = lyricEntry{text: text}
	}
	return entries
}

func visibleRows(rows []lyricRow) []string {
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = ansi.Strip(row.text)
	}
	return out
}

func assertRows(t *testing.T, got []lyricRow, want []string) {
	t.Helper()
	vis := visibleRows(got)
	if len(vis) != len(want) {
		t.Fatalf("got %d rows %q, want %d %q", len(vis), vis, len(want), want)
	}
	for i := range want {
		if vis[i] != want[i] {
			t.Fatalf("row %d = %q, want %q\nall: %q", i, vis[i], want[i], vis)
		}
	}
}

func useEmptyConfig(t *testing.T) {
	t.Helper()
	previous := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	t.Cleanup(func() { configs.AppConfig = previous })
}

func TestLayoutLyricRowsInlineWhenFits(t *testing.T) {
	useEmptyConfig(t)
	entries := []lyricEntry{{text: "a", trans: "A"}, {text: "b", trans: "B"}, {text: "c"}}
	rows := layoutLyricRows(entries, 1, 3, false)
	assertRows(t, rows, []string{"a [A]", "b [B]", "c"})
	if !rows[1].highlight || rows[0].highlight || rows[2].highlight {
		t.Fatalf("only the middle row should be highlighted: %+v", rows)
	}
}

func TestLayoutLyricRowsStacksTranslationWhenTooWide(t *testing.T) {
	useEmptyConfig(t)
	entries := []lyricEntry{
		{text: "prev", trans: "上一句很长的翻译"},
		{text: "current", trans: "当前这一句很长的翻译"},
		{text: "next", trans: "下一句很长的翻译"},
		{text: "after"},
		{text: "last"},
	}
	rows := layoutLyricRows(entries, 1, 7, true)
	assertRows(t, rows, []string{
		"",
		"prev",
		"上一句很长的翻译",
		"current",
		"当前这一句很长的翻译",
		"next",
		"下一句很长的翻译",
	})
	if !rows[3].highlight || rows[4].highlight {
		t.Fatalf("highlight must be the current main text only: %+v", rows)
	}
}

func TestLayoutLyricRowsAboveKeepsMainTextWhenCut(t *testing.T) {
	useEmptyConfig(t)
	entries := []lyricEntry{{text: "prev", trans: "很长很长的翻译"}, {text: "cur"}, {text: "next"}}
	// Only one row above the middle: the previous line keeps its text.
	assertRows(t, layoutLyricRows(entries, 1, 3, true), []string{"prev", "cur", "next"})
}

func TestLayoutLyricRowsCurrentStackedInSmallBlock(t *testing.T) {
	useEmptyConfig(t)
	entries := []lyricEntry{{text: "prev"}, {text: "current", trans: "很长很长的翻译"}, {text: "next"}}
	assertRows(t, layoutLyricRows(entries, 1, 3, true), []string{"prev", "current", "很长很长的翻译"})
}

func TestSongInlineWidth(t *testing.T) {
	state := lyric.State{
		ShowTranslation:     true,
		Fragments:           []lyric.LRCFragment{{StartTimeMs: 0, Content: "ab"}, {StartTimeMs: 1, Content: "abcdef"}, {StartTimeMs: 2, Content: "abcdefghij"}},
		TranslatedFragments: map[int64]string{0: "中文", 1: "x"},
	}
	// "ab [中文]" = 2+3+4 = 9; "abcdef [x]" = 6+4 = 10; line 2 has no translation.
	if got := songInlineWidth(state); got != 10 {
		t.Fatalf("songInlineWidth = %d, want 10", got)
	}
	state.ShowTranslation = false
	if got := songInlineWidth(state); got != 0 {
		t.Fatalf("hidden translation width = %d, want 0", got)
	}
	yrc := lyric.State{ShowTranslation: true, YRCEnabled: true, YRCLines: []lyric.YRCLine{
		{Words: []lyric.YRCWord{{Word: "甲"}, {Word: "乙"}}, TranslatedLyric: "ab"},
	}}
	// "甲乙 [ab]" = 4+5 = 9
	if got := songInlineWidth(yrc); got != 9 {
		t.Fatalf("YRC songInlineWidth = %d, want 9", got)
	}
}

func TestLyricLineCount(t *testing.T) {
	tests := []struct{ space, max, want int }{
		{3, 5, 3},
		{4, 5, 3},
		{5, 5, 5},
		{40, 5, 5}, // default keeps upstream behaviour
		{40, 11, 11},
		{8, 11, 7}, // shrinks to fit, stays odd
		{40, 12, 11},
	}
	for _, test := range tests {
		if got := lyricLineCount(test.space, test.max); got != test.want {
			t.Errorf("lyricLineCount(%d, %d) = %d, want %d", test.space, test.max, got, test.want)
		}
	}
}

func TestLyricMaxLines(t *testing.T) {
	useEmptyConfig(t)
	if got := lyricMaxLines(); got != FullLyricLines {
		t.Fatalf("unset maxLines = %d, want %d", got, FullLyricLines)
	}
	configs.AppConfig.Main.Lyric.MaxLines = 11
	if got := lyricMaxLines(); got != 11 {
		t.Fatalf("maxLines = %d, want 11", got)
	}
	configs.AppConfig.Theme.DynamicMenuRows = true
	if got := lyricMaxLines(); got != FullLyricLines {
		t.Fatalf("dynamic menu rows must keep %d, got %d", FullLyricLines, got)
	}
}
