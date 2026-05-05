// Package ui provides simple tcell-based screens (file picker, message boxes, summary).
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
)

var audioExts = map[string]bool{
	".mp3":  true,
	".wav":  true,
	".ogg":  true,
	".flac": true,
}

type entry struct {
	name  string
	isDir bool
	full  string
}

// PickResult tells the caller what the user chose.
type PickResult struct {
	Cancel    bool
	Path      string // absolute path of selected audio file (empty if URL)
	URL       string // user-entered URL (mutually exclusive with Path)
	Dir       string // last visited dir (for persisting)
	Reanalyze bool   // user pressed `r` on the highlighted file
}

// PickFile presents a file picker. Returns selected audio path or Cancel=true.
func PickFile(scr tcell.Screen, startDir string) (PickResult, error) {
	dir := startDir
	if d, err := filepath.Abs(dir); err == nil {
		dir = d
	}
	cursor := 0
	scrollOff := 0

	for {
		entries, err := listDir(dir)
		if err != nil {
			return PickResult{}, err
		}
		if cursor >= len(entries) {
			cursor = len(entries) - 1
		}
		if cursor < 0 {
			cursor = 0
		}

		drawPicker(scr, dir, entries, cursor, &scrollOff)

		ev := scr.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				return PickResult{Cancel: true, Dir: dir}, nil
			case tcell.KeyUp:
				if cursor > 0 {
					cursor--
				}
			case tcell.KeyDown:
				if cursor < len(entries)-1 {
					cursor++
				}
			case tcell.KeyPgUp:
				cursor -= 10
				if cursor < 0 {
					cursor = 0
				}
			case tcell.KeyPgDn:
				cursor += 10
				if cursor > len(entries)-1 {
					cursor = len(entries) - 1
				}
			case tcell.KeyHome:
				cursor = 0
			case tcell.KeyEnd:
				cursor = len(entries) - 1
			case tcell.KeyEnter:
				if len(entries) == 0 {
					continue
				}
				e := entries[cursor]
				if e.isDir {
					dir = e.full
					cursor = 0
					scrollOff = 0
				} else {
					return PickResult{Path: e.full, Dir: dir}, nil
				}
			default:
				switch ev.Rune() {
				case 'q', 'Q':
					return PickResult{Cancel: true, Dir: dir}, nil
				case 'r', 'R':
					if len(entries) > 0 {
						e := entries[cursor]
						if !e.isDir {
							return PickResult{Path: e.full, Dir: dir, Reanalyze: true}, nil
						}
					}
				case 'u', 'U':
					url, ok := PromptLine(scr, "Enter URL (YouTube etc.):")
					if ok && strings.TrimSpace(url) != "" {
						return PickResult{URL: strings.TrimSpace(url), Dir: dir}, nil
					}
				case 'h', 'H':
					home, _ := os.UserHomeDir()
					dir = home
					cursor = 0
					scrollOff = 0
				case 'k':
					if cursor > 0 {
						cursor--
					}
				case 'j':
					if cursor < len(entries)-1 {
						cursor++
					}
				}
			}
		}
	}
}

func listDir(dir string) ([]entry, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []entry{}
	parent := filepath.Dir(dir)
	if parent != dir {
		out = append(out, entry{name: "../", isDir: true, full: parent})
	}
	dirs, audios := []entry{}, []entry{}
	for _, f := range files {
		name := f.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if f.IsDir() {
			dirs = append(dirs, entry{name: name + "/", isDir: true, full: full})
		} else {
			ext := strings.ToLower(filepath.Ext(name))
			if audioExts[ext] {
				audios = append(audios, entry{name: name, full: full})
			}
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].name < dirs[j].name })
	sort.Slice(audios, func(i, j int) bool { return audios[i].name < audios[j].name })
	out = append(out, dirs...)
	out = append(out, audios...)
	return out, nil
}

func drawPicker(scr tcell.Screen, dir string, entries []entry, cursor int, scrollOff *int) {
	scr.Clear()
	w, h := scr.Size()
	plain := tcell.StyleDefault
	bold := plain.Bold(true)
	dim := plain.Foreground(tcell.ColorGray)

	drawString(scr, 0, 0, " TempoTTY  ♪  pick a song", bold.Foreground(tcell.ColorAqua))
	drawString(scr, 0, 2, "  "+truncate(dir, w-4), plain.Foreground(tcell.ColorYellow))

	listTop := 4
	listH := h - listTop - 2
	if listH < 1 {
		listH = 1
	}

	if cursor < *scrollOff {
		*scrollOff = cursor
	}
	if cursor >= *scrollOff+listH {
		*scrollOff = cursor - listH + 1
	}

	if len(entries) == 0 {
		drawString(scr, 2, listTop, "(no audio files in this directory)", dim)
	}
	for i := 0; i < listH; i++ {
		idx := *scrollOff + i
		if idx >= len(entries) {
			break
		}
		e := entries[idx]
		marker := "  "
		st := plain
		if idx == cursor {
			marker = "▶ "
			if e.isDir {
				st = bold.Foreground(tcell.ColorAqua).Reverse(true)
			} else {
				st = bold.Foreground(tcell.ColorYellow).Reverse(true)
			}
		} else if e.isDir {
			st = plain.Foreground(tcell.ColorAqua)
		}
		line := marker + e.name
		drawString(scr, 0, listTop+i, padRight(line, w), st)
	}

	help := " ↑↓/jk move   Enter open   u URL   r re-analyze   h home   q quit "
	drawString(scr, 0, h-1, padRight(help, w), dim.Reverse(true))
	scr.Show()
}

// PromptLine displays a single-line text input. Returns (text, true) on Enter,
// or ("", false) on Esc.
func PromptLine(scr tcell.Screen, label string) (string, bool) {
	buf := []rune{}
	for {
		drawPrompt(scr, label, buf)
		ev := scr.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventKey:
			switch ev.Key() {
			case tcell.KeyEscape, tcell.KeyCtrlC:
				return "", false
			case tcell.KeyEnter:
				return string(buf), true
			case tcell.KeyBackspace, tcell.KeyBackspace2:
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
				}
			case tcell.KeyCtrlU:
				buf = buf[:0]
			case tcell.KeyCtrlW:
				// delete back to whitespace
				for len(buf) > 0 && buf[len(buf)-1] == ' ' {
					buf = buf[:len(buf)-1]
				}
				for len(buf) > 0 && buf[len(buf)-1] != ' ' {
					buf = buf[:len(buf)-1]
				}
			default:
				r := ev.Rune()
				if r >= 0x20 && r != 0x7f {
					buf = append(buf, r)
				}
			}
		}
	}
}

func drawPrompt(scr tcell.Screen, label string, buf []rune) {
	scr.Clear()
	w, h := scr.Size()
	plain := tcell.StyleDefault
	bold := plain.Bold(true)
	dim := plain.Foreground(tcell.ColorGray)

	drawString(scr, 0, 0, " TempoTTY  ♪  paste a URL", bold.Foreground(tcell.ColorAqua))
	drawString(scr, 2, h/2-1, label, plain)
	box := "  > " + string(buf) + "█"
	drawString(scr, 2, h/2, padRight(box, w-2), bold.Foreground(tcell.ColorYellow))
	drawString(scr, 0, h-1, padRight(" Enter accept   Esc cancel   Ctrl-U clear ", w), dim.Reverse(true))
	scr.Show()
}

// Message shows a centered status line and returns immediately (no input).
func Message(scr tcell.Screen, msg string) {
	scr.Clear()
	w, h := scr.Size()
	plain := tcell.StyleDefault.Bold(true)
	x := (w - len(msg)) / 2
	if x < 0 {
		x = 0
	}
	drawString(scr, x, h/2, msg, plain)
	scr.Show()
}

// Result shows the result screen and waits for any key. Returns true to continue, false to quit.
func Result(scr tcell.Screen, title string, lines []string, footer string) bool {
	for {
		scr.Clear()
		w, h := scr.Size()
		plain := tcell.StyleDefault
		bold := plain.Bold(true)

		x := (w - len(title)) / 2
		if x < 0 {
			x = 0
		}
		drawString(scr, x, 2, title, bold.Foreground(tcell.ColorYellow))
		for i, l := range lines {
			lx := (w - len(l)) / 2
			if lx < 0 {
				lx = 0
			}
			drawString(scr, lx, 5+i, l, plain)
		}
		fy := h - 2
		fx := (w - len(footer)) / 2
		if fx < 0 {
			fx = 0
		}
		drawString(scr, fx, fy, footer, plain.Foreground(tcell.ColorAqua))
		scr.Show()

		ev := scr.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			scr.Sync()
		case *tcell.EventKey:
			r := ev.Rune()
			if ev.Key() == tcell.KeyEscape || r == 'q' || r == 'Q' {
				return false
			}
			return true
		}
	}
}

func drawString(scr tcell.Screen, x, y int, str string, st tcell.Style) {
	w, _ := scr.Size()
	for _, r := range str {
		if x >= w {
			return
		}
		scr.SetContent(x, y, r, nil, st)
		x++
	}
}

func padRight(s string, w int) string {
	if len(s) >= w {
		return s[:w]
	}
	return s + strings.Repeat(" ", w-len(s))
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	if w < 4 {
		return s[:w]
	}
	return "…" + s[len(s)-w+1:]
}

// Compile-time check we can format ints (avoid unused import if reorganized)
var _ = fmt.Sprintf
