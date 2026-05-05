// Package game runs a rhythm-game session on a caller-provided tcell screen.
package game

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/speaker"

	"tempo-tty/internal/audio"
	"tempo-tty/internal/chart"
)

type judgement struct {
	label  string
	window float64
	score  int
}

var judgements = []judgement{
	{"PERFECT", 0.05, 300},
	{"GREAT", 0.10, 200},
	{"GOOD", 0.15, 100},
}

const (
	missAfter = 0.15
	laneRows  = 3
	judgeCol  = 6
	offsetStep = 0.005
)

type Options struct {
	Speed  float64
	Offset float64
	Keys   string
}

type Result struct {
	Quit      bool
	Score     int
	MaxCombo  int
	Counts    map[string]int
	Accuracy  float64
	NewOffset float64 // updated by live `[`/`]` adjustment
}

type note struct {
	t      float64
	lane   int
	judged bool
}

type session struct {
	screen tcell.Screen
	c      *chart.Chart
	notes  []note
	keys   []rune
	keyMap map[rune]int

	speed  float64
	offset float64

	score, combo, maxCombo int
	counts                 map[string]int
	lastJudge              string
	lastJudgeAt            time.Time

	laneNotes [][]int
	nextIdx   []int

	clock *audio.Counter
}

// Run plays one chart on the given screen. Caller owns screen lifetime.
func Run(scr tcell.Screen, c *chart.Chart, opts Options) (Result, error) {
	streamer, format, err := audio.Decode(c.Audio)
	if err != nil {
		return Result{}, fmt.Errorf("decode %s: %w", c.Audio, err)
	}
	defer streamer.Close()

	if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Millisecond*50)); err != nil {
		return Result{}, err
	}
	defer speaker.Close()

	clock := &audio.Counter{S: streamer, SampleRate: int(format.SampleRate)}
	doneCh := make(chan struct{})
	speaker.Play(beep.Seq(clock, beep.Callback(func() { close(doneCh) })))

	keys := opts.Keys
	if len(keys) < c.Lanes {
		keys = (keys + "dfjkasl;qweruiop")[:c.Lanes]
	}
	keyRunes := []rune(strings.ToLower(keys[:c.Lanes]))
	keyMap := make(map[rune]int, c.Lanes)
	for i, r := range keyRunes {
		keyMap[r] = i
	}

	notes := make([]note, len(c.Notes))
	laneNotes := make([][]int, c.Lanes)
	for i, n := range c.Notes {
		notes[i] = note{t: n.T, lane: n.Lane}
		if n.Lane >= 0 && n.Lane < c.Lanes {
			laneNotes[n.Lane] = append(laneNotes[n.Lane], i)
		}
	}
	for _, l := range laneNotes {
		sort.SliceStable(l, func(i, j int) bool { return notes[l[i]].t < notes[l[j]].t })
	}

	s := &session{
		screen: scr, c: c, notes: notes,
		keys: keyRunes, keyMap: keyMap,
		speed: opts.Speed, offset: opts.Offset,
		counts:    map[string]int{"PERFECT": 0, "GREAT": 0, "GOOD": 0, "MISS": 0},
		laneNotes: laneNotes,
		nextIdx:   make([]int, c.Lanes),
		clock:     clock,
	}

	events := make(chan tcell.Event, 64)
	stop := make(chan struct{})
	go func() {
		for {
			ev := scr.PollEvent()
			if ev == nil {
				return
			}
			select {
			case events <- ev:
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)

	const frame = 16 * time.Millisecond
	ticker := time.NewTicker(frame)
	defer ticker.Stop()

	res := Result{}

	for {
		audioT := s.clock.Position() + s.offset

	drain:
		for {
			select {
			case ev := <-events:
				switch ev := ev.(type) {
				case *tcell.EventKey:
					if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyCtrlC {
						res.Quit = true
						return s.finish(res), nil
					}
					r := ev.Rune()
					if r == 'q' || r == 'Q' {
						res.Quit = true
						return s.finish(res), nil
					}
					switch r {
					case '[':
						s.offset -= offsetStep
					case ']':
						s.offset += offsetStep
					case '\\':
						s.offset = 0
					default:
						if lane, ok := s.keyMap[r]; ok {
							s.handleHit(lane, audioT)
						}
					}
				case *tcell.EventResize:
					scr.Sync()
				}
			default:
				break drain
			}
		}

		s.processMisses(audioT)

		if audioT > s.c.Duration+1.0 {
			return s.finish(res), nil
		}
		select {
		case <-doneCh:
			return s.finish(res), nil
		default:
		}

		s.render(audioT)
		<-ticker.C
	}
}

func (s *session) finish(res Result) Result {
	speaker.Clear()
	res.Score = s.score
	res.MaxCombo = s.maxCombo
	res.Counts = s.counts
	res.NewOffset = s.offset
	total := 0
	for _, n := range s.counts {
		total += n
	}
	if total == 0 {
		total = 1
	}
	res.Accuracy = (float64(s.counts["PERFECT"])*1.0 +
		float64(s.counts["GREAT"])*0.7 +
		float64(s.counts["GOOD"])*0.4) / float64(total) * 100
	return res
}

func (s *session) handleHit(lane int, audioT float64) {
	for j := s.nextIdx[lane]; j < len(s.laneNotes[lane]); j++ {
		idx := s.laneNotes[lane][j]
		n := &s.notes[idx]
		if n.judged {
			continue
		}
		delta := audioT - n.t
		if delta < -missAfter {
			return
		}
		for _, jd := range judgements {
			if math.Abs(delta) <= jd.window {
				n.judged = true
				bonus := 1.0 + float64(s.combo)/25.0
				s.score += int(float64(jd.score) * bonus)
				s.combo++
				if s.combo > s.maxCombo {
					s.maxCombo = s.combo
				}
				s.counts[jd.label]++
				s.lastJudge = jd.label
				s.lastJudgeAt = time.Now()
				return
			}
		}
		if delta > missAfter {
			continue
		}
		return
	}
}

func (s *session) processMisses(audioT float64) {
	for lane := 0; lane < s.c.Lanes; lane++ {
		for s.nextIdx[lane] < len(s.laneNotes[lane]) {
			idx := s.laneNotes[lane][s.nextIdx[lane]]
			n := &s.notes[idx]
			if n.judged {
				s.nextIdx[lane]++
				continue
			}
			if audioT-n.t > missAfter {
				n.judged = true
				s.counts["MISS"]++
				s.combo = 0
				s.lastJudge = "MISS"
				s.lastJudgeAt = time.Now()
				s.nextIdx[lane]++
			} else {
				break
			}
		}
	}
}

func (s *session) render(audioT float64) {
	scr := s.screen
	w, _ := scr.Size()
	scr.Clear()

	plain := tcell.StyleDefault
	dim := plain.Foreground(tcell.ColorGray)
	bold := plain.Bold(true)

	title := fmt.Sprintf(" TempoTTY  ♪ %s   BPM %.1f   %s / %s",
		filepath.Base(s.c.Audio), s.c.BPM, fmtTime(audioT), fmtTime(s.c.Duration))
	right := fmt.Sprintf("Score %d  Combo x%d  Best x%d  off:%+.3f ",
		s.score, s.combo, s.maxCombo, s.offset)
	drawStr(scr, 0, 0, title, bold)
	if w-len(right) > 0 {
		drawStr(scr, w-len(right), 0, right, bold)
	}

	pw := w - 2
	if pw < 4 {
		pw = 4
	}
	prog := clamp01(audioT / math.Max(s.c.Duration, 0.1))
	filled := int(float64(pw) * prog)
	for x := 0; x < pw; x++ {
		r := '·'
		st := dim
		if x < filled {
			r = '─'
			st = plain
		}
		scr.SetContent(1+x, 1, r, nil, st)
	}

	fieldTop := 3
	fieldBottom := fieldTop + s.c.Lanes*laneRows
	for x := 0; x < w; x++ {
		scr.SetContent(x, fieldTop-1, '─', nil, dim)
		scr.SetContent(x, fieldBottom, '─', nil, dim)
	}

	flight := s.speed
	fieldW := w - judgeCol - 2
	if fieldW < 1 {
		fieldW = 1
	}

	cyan := plain.Foreground(tcell.ColorAqua)
	for lane := 0; lane < s.c.Lanes; lane++ {
		yMid := fieldTop + lane*laneRows + laneRows/2
		drawStr(scr, 1, yMid, strings.ToUpper(string(s.keys[lane])), bold.Foreground(tcell.ColorYellow))
		for r := 0; r < laneRows; r++ {
			y := fieldTop + lane*laneRows + r
			scr.SetContent(judgeCol, y, '┃', nil, cyan)
		}
		for x := judgeCol + 1; x < w-1; x++ {
			scr.SetContent(x, yMid, '·', nil, dim)
		}
	}

	trail := []rune{'◆', '◇', '·'}
	trailColors := []tcell.Color{tcell.ColorSilver, tcell.ColorGray, tcell.ColorDarkGray}
	for i := range s.notes {
		n := &s.notes[i]
		if n.judged {
			continue
		}
		dt := n.t - audioT
		if dt > flight || dt < -0.2 {
			continue
		}
		x := judgeCol + int((dt/flight)*float64(fieldW))
		if x < 0 || x >= w {
			continue
		}
		y := fieldTop + n.lane*laneRows + laneRows/2

		for k, r := range trail {
			tx := x + k + 1
			if tx >= w-1 {
				break
			}
			scr.SetContent(tx, y, r, nil, plain.Foreground(trailColors[k]))
		}
		var st tcell.Style
		switch {
		case math.Abs(dt) < 0.05:
			st = bold.Foreground(tcell.ColorYellow)
		case math.Abs(dt) < 0.15:
			st = bold.Foreground(tcell.ColorLime)
		default:
			st = bold.Foreground(tcell.ColorWhite)
		}
		scr.SetContent(x, y, '●', nil, st)
	}

	if s.lastJudge != "" && time.Since(s.lastJudgeAt) < 400*time.Millisecond {
		var col tcell.Color
		switch s.lastJudge {
		case "PERFECT":
			col = tcell.ColorYellow
		case "GREAT":
			col = tcell.ColorLime
		case "GOOD":
			col = tcell.ColorAqua
		default:
			col = tcell.ColorRed
		}
		drawStr(scr, judgeCol-2, fieldTop-2, "  "+s.lastJudge+"  ", bold.Foreground(col))
	}

	upperKeys := make([]string, len(s.keys))
	for i, r := range s.keys {
		upperKeys[i] = strings.ToUpper(string(r))
	}
	info := fmt.Sprintf(" Keys: %s   [/] offset∓5ms   \\ reset   [Q]uit   P:%d G:%d g:%d M:%d ",
		strings.Join(upperKeys, " "),
		s.counts["PERFECT"], s.counts["GREAT"], s.counts["GOOD"], s.counts["MISS"])
	drawStr(scr, 0, fieldBottom+1, truncate(info, w), plain)

	scr.Show()
}

func drawStr(scr tcell.Screen, x, y int, str string, st tcell.Style) {
	w, _ := scr.Size()
	for _, r := range str {
		if x >= w {
			return
		}
		scr.SetContent(x, y, r, nil, st)
		x++
	}
}

func fmtTime(s float64) string {
	if s < 0 {
		s = 0
	}
	t := int(s)
	return fmt.Sprintf("%02d:%02d", t/60, t%60)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func truncate(s string, w int) string {
	if len(s) <= w {
		return s
	}
	return s[:w]
}
