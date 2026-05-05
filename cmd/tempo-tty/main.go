// tempo-tty is the integrated TUI: pick a song, auto-analyze, play, save offset.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gdamore/tcell/v2"

	"tempo-tty/internal/analyze"
	"tempo-tty/internal/chart"
	"tempo-tty/internal/config"
	"tempo-tty/internal/game"
	"tempo-tty/internal/source"
	"tempo-tty/internal/ui"
)

func main() {
	cfg := config.Load()

	scr, err := tcell.NewScreen()
	if err != nil {
		fail(err)
	}
	if err := scr.Init(); err != nil {
		fail(err)
	}
	defer scr.Fini()
	scr.HideCursor()

	startDir, err := os.Getwd()
	if err != nil || startDir == "" {
		startDir = cfg.LastDir
	}

	// CLI shortcut: tempo-tty <url-or-path>
	if len(os.Args) >= 2 {
		input := os.Args[1]
		audioPath, err := resolveSource(scr, input)
		if err != nil {
			scr.Fini()
			fail(err)
		}
		runSong(scr, audioPath, false, cfg)
		return
	}

	for {
		pick, err := ui.PickFile(scr, startDir)
		if err != nil {
			scr.Fini()
			fail(err)
		}
		startDir = pick.Dir
		if pick.Cancel {
			return
		}

		var audioPath string
		if pick.URL != "" {
			audioPath, err = resolveSource(scr, pick.URL)
			if err != nil {
				showError(scr, err.Error())
				continue
			}
		} else {
			audioPath = pick.Path
		}

		if cont := runSong(scr, audioPath, pick.Reanalyze, cfg); !cont {
			return
		}
	}
}

// runSong analyzes (if needed), plays, shows result. Returns false if user wants to quit.
func runSong(scr tcell.Screen, audioPath string, force bool, cfg *config.Config) bool {
	c, err := loadOrAnalyze(scr, audioPath, cfg, force)
	if err != nil {
		showError(scr, err.Error())
		return true
	}

	res, err := game.Run(scr, c, game.Options{
		Speed:  cfg.Speed,
		Offset: cfg.Offset,
		Keys:   cfg.Keys,
	})
	if err != nil {
		showError(scr, err.Error())
		return true
	}

	if res.NewOffset != cfg.Offset {
		cfg.Offset = res.NewOffset
		_ = config.Save(cfg)
	}

	title := "=== Result ==="
	if res.Quit {
		title = "=== Quit ==="
	}
	lines := []string{
		fmt.Sprintf("Score:      %d", res.Score),
		fmt.Sprintf("Max Combo:  %d", res.MaxCombo),
		fmt.Sprintf("PERFECT %d   GREAT %d   GOOD %d   MISS %d",
			res.Counts["PERFECT"], res.Counts["GREAT"],
			res.Counts["GOOD"], res.Counts["MISS"]),
		fmt.Sprintf("Accuracy:   %.2f%%", res.Accuracy),
		"",
		fmt.Sprintf("Offset saved: %+.3fs", cfg.Offset),
	}
	return ui.Result(scr, title, lines, "[Enter] back to picker     [Q] quit")
}

func resolveSource(scr tcell.Screen, input string) (string, error) {
	if !source.IsURL(input) {
		return source.Resolve(input, nil)
	}

	type out struct {
		path string
		err  error
	}
	ch := make(chan out, 1)
	statusCh := make(chan string, 8)

	go func() {
		p, err := source.Resolve(input, func(s string) {
			select {
			case statusCh <- s:
			default:
			}
		})
		ch <- out{p, err}
	}()

	frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	t := time.NewTicker(120 * time.Millisecond)
	defer t.Stop()
	i := 0
	status := "downloading…"
	for {
		select {
		case r := <-ch:
			return r.path, r.err
		case s := <-statusCh:
			status = s
		case <-t.C:
			ui.Message(scr, fmt.Sprintf("%c  %s", frames[i%len(frames)], status))
			i++
		}
	}
}

func loadOrAnalyze(scr tcell.Screen, audioPath string, cfg *config.Config, force bool) (*chart.Chart, error) {
	chartPath := analyze.ChartPathFor(audioPath)

	if !force {
		if c, err := chart.Load(chartPath); err == nil {
			c.Audio = audioPath
			return c, nil
		}
	}

	ui.Message(scr, fmt.Sprintf("Analyzing %s …", filepath.Base(audioPath)))

	type out struct {
		c   *chart.Chart
		err error
	}
	ch := make(chan out, 1)
	go func() {
		c, err := analyze.Run(audioPath, analyze.Options{
			Lanes:  cfg.Lanes,
			MinGap: cfg.MinGap,
		})
		ch <- out{c, err}
	}()

	frames := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	i := 0
	for {
		select {
		case r := <-ch:
			if r.err != nil {
				return nil, r.err
			}
			r.c.Audio = audioPath
			if err := chart.Save(chartPath, r.c); err != nil {
				_ = err
			}
			return r.c, nil
		case <-t.C:
			ui.Message(scr, fmt.Sprintf("%c  Analyzing %s …", frames[i%len(frames)], filepath.Base(audioPath)))
			i++
		}
	}
}

func showError(scr tcell.Screen, msg string) {
	ui.Result(scr, "Error", []string{msg}, "[Enter] back   [Q] quit")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
