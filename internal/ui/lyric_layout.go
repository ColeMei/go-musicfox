package ui

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/go-musicfox/go-musicfox/internal/configs"
	"github.com/go-musicfox/go-musicfox/internal/lyric"
)

// lyricEntry is one lyric line plus its optional translation. Layout decides
// whether the translation fits inline or moves to its own row.
type lyricEntry struct {
	text  string // may contain ANSI styling
	trans string // plain text, empty when hidden or missing
}

// lyricRow is one rendered terminal row of the lyric block.
type lyricRow struct {
	text      string // may contain ANSI styling
	highlight bool   // main text of the current line
}

func yrcTranslation(line lyric.YRCLine, show bool) string {
	if !show {
		return ""
	}
	return line.TranslatedLyric
}

// songInlineWidth is the widest "text [trans]" line of the song, or 0 when no
// translation is shown. Deciding stacking per song (not per visible block)
// keeps the layout stable while lyrics scroll.
func songInlineWidth(state lyric.State) int {
	if !state.ShowTranslation {
		return 0
	}
	widest := 0
	measure := func(text, trans string) {
		if trans != "" {
			widest = max(widest, runewidth.StringWidth(text)+runewidth.StringWidth(" ["+trans+"]"))
		}
	}
	if state.YRCEnabled && len(state.YRCLines) > 0 {
		for _, line := range state.YRCLines {
			var text strings.Builder
			for _, word := range line.Words {
				text.WriteString(word.Word)
			}
			measure(text.String(), line.TranslatedLyric)
		}
		return widest
	}
	for _, frag := range state.Fragments {
		measure(frag.Content, state.TranslatedFragments[frag.StartTimeMs])
	}
	return widest
}

// entryRows lays out one entry: "text [trans]" on one row, or the translation
// on the row below when stack is set.
func entryRows(e lyricEntry, current, stack bool) []lyricRow {
	main := lyricRow{text: e.text, highlight: current}
	if e.trans == "" {
		return []lyricRow{main}
	}
	if !stack {
		main.text += styleLyricText(" ["+e.trans+"]", lyricInactiveColor())
		return []lyricRow{main}
	}
	return []lyricRow{main, {text: styleLyricText(e.trans, lyricInactiveColor())}}
}

// layoutLyricRows fills rows terminal rows with entries. entries[center] is
// the current line; its first row sits on the middle row, and neighbours fill
// outward until rows run out. An entry cut by the edge keeps its main text.
func layoutLyricRows(entries []lyricEntry, center, rows int, stack bool) []lyricRow {
	out := make([]lyricRow, rows)
	if rows <= 0 || center < 0 || center >= len(entries) {
		return out
	}
	mid := (rows - 1) / 2

	pos := mid
	for _, row := range entryRows(entries[center], true, stack) {
		if pos >= rows {
			break
		}
		out[pos] = row
		pos++
	}
	for i := center + 1; i < len(entries) && pos < rows; i++ {
		for _, row := range entryRows(entries[i], false, stack) {
			if pos >= rows {
				break
			}
			out[pos] = row
			pos++
		}
	}

	pos = mid - 1
	for i := center - 1; i >= 0 && pos >= 0; i-- {
		er := entryRows(entries[i], false, stack)
		if len(er) > pos+1 {
			er = er[:1]
		}
		for j := len(er) - 1; j >= 0; j-- {
			out[pos] = er[j]
			pos--
		}
	}
	return out
}

// lyricMaxLines is the configured cap on lyric rows. Dynamic menu mode budgets
// a fixed FullLyricLines block, so it keeps the original cap.
func lyricMaxLines() int {
	if configs.AppConfig == nil || configs.AppConfig.Theme.DynamicMenuRows {
		return FullLyricLines
	}
	if n := configs.AppConfig.Main.Lyric.MaxLines; n >= CompactLyricLines {
		return n
	}
	return FullLyricLines
}

// lyricLineCount picks an odd row count (so the current line stays centred)
// that fits spaceHeight, between CompactLyricLines and maxLines.
func lyricLineCount(spaceHeight, maxLines int) int {
	n := min(spaceHeight, maxLines)
	if n%2 == 0 {
		n--
	}
	return max(n, CompactLyricLines)
}
