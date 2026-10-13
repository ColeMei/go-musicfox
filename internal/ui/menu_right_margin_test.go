package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/anhoder/foxful-cli/model"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/go-musicfox/go-musicfox/internal/configs"
)

type longSubtitleMenu struct {
	model.DefaultMenu
}

func (longSubtitleMenu) GetMenuKey() string { return "long-subtitle" }

func (longSubtitleMenu) MenuViews() []model.MenuItem {
	return []model.MenuItem{
		{Title: "Multiply", Subtitle: "Bren Joy"},
		{Title: "Blueberry Faygo", Subtitle: "Lil Mosey"},
		{Title: "223's", Subtitle: "YNW Melly,9lokknine"},
		{Title: "MAYNARD VIGNETTE", Subtitle: "cquees,Killer Mike,T.I.,Lil Wayne"},
		{Title: "run!", Subtitle: "nafla,JUSTHIS"},
		{Title: "Like That", Subtitle: "Future,Metro Boomin,Kendrick Lamar"},
	}
}

func (longSubtitleMenu) HelpHints() []model.HelpHint { return nil }

// The right column of a wide dual-column menu must stop short of the window
// edge instead of running into it (go-musicfox fork patch in foxful-cli).
func TestDualColumnRightColumnKeepsMargin(t *testing.T) {
	const width, height = 120, 40

	previousConfig := configs.AppConfig
	configs.AppConfig = &configs.Config{}
	configs.AppConfig.Theme.ShowTitle = true
	t.Cleanup(func() { configs.AppConfig = previousConfig })

	opts := model.DefaultOptions()
	opts.EnableStartup = false
	opts.WhetherDisplayTitle = true
	opts.DualColumn = true
	opts.MainMenu = &longSubtitleMenu{}
	opts.MainMenuTitle = &model.MenuItem{Title: "当前播放列表"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opts.TeaOptions = []tea.ProgramOption{tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard)}
	app := model.NewApp(opts)
	_ = app.Run()
	_, _ = app.Update(tea.WindowSizeMsg{Width: width, Height: height})

	view := ansi.Strip(app.View().Content)
	found := false
	for _, line := range strings.Split(view, "\n") {
		if !strings.Contains(line, "Like That") && !strings.Contains(line, "MAYNARD") {
			continue
		}
		found = true
		if w := runewidth.StringWidth(strings.TrimRight(line, " ")); w > width-4 {
			t.Fatalf("right column reaches column %d of %d, want <= %d:\n%q", w, width, width-4, line)
		}
	}
	if !found {
		t.Fatalf("menu rows not rendered:\n%s", view)
	}
}
