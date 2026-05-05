// Package analyze wraps onset detection + chart writing for reuse from CLI and TUI.
package analyze

import (
	"math"
	"path/filepath"

	"tempo-tty/internal/audio"
	"tempo-tty/internal/chart"
	"tempo-tty/internal/onset"
)

type Options struct {
	Lanes  int
	MinGap float64
}

// Run loads audio, detects onsets, returns a chart (does not write to disk).
func Run(audioPath string, opts Options) (*chart.Chart, error) {
	abs, err := filepath.Abs(audioPath)
	if err != nil {
		return nil, err
	}
	samples, sr, err := audio.LoadMono(audioPath)
	if err != nil {
		return nil, err
	}
	duration := float64(len(samples)) / float64(sr)

	times, cents := onset.Detect(samples, sr, opts.MinGap)
	laneIdx := onset.AssignLanes(cents, opts.Lanes)

	notes := make([]chart.Note, len(times))
	for i := range times {
		notes[i] = chart.Note{T: round(times[i], 4), Lane: laneIdx[i]}
	}
	return &chart.Chart{
		Audio:    abs,
		Duration: round(duration, 3),
		BPM:      round(onset.EstimateBPM(times), 2),
		Lanes:    opts.Lanes,
		Notes:    notes,
	}, nil
}

// ChartPathFor returns the cached chart path for an audio file (next to it).
func ChartPathFor(audioPath string) string {
	abs, err := filepath.Abs(audioPath)
	if err != nil {
		abs = audioPath
	}
	ext := filepath.Ext(abs)
	return abs[:len(abs)-len(ext)] + ".chart.json"
}

func round(v float64, n int) float64 {
	p := math.Pow(10, float64(n))
	return math.Round(v*p) / p
}
