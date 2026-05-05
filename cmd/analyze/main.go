// analyze is a thin CLI wrapper around internal/analyze.
package main

import (
	"flag"
	"fmt"
	"os"

	"tempo-tty/internal/analyze"
	"tempo-tty/internal/chart"
)

func main() {
	out := flag.String("o", "chart.json", "output chart.json path")
	lanes := flag.Int("lanes", 4, "number of lanes")
	minGap := flag.Float64("min-gap", 0.08, "minimum seconds between consecutive notes")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: analyze [flags] <audio-file>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}
	audioPath := flag.Arg(0)

	fmt.Fprintf(os.Stderr, "[analyze] decoding %s…\n", audioPath)
	c, err := analyze.Run(audioPath, analyze.Options{Lanes: *lanes, MinGap: *minGap})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := chart.Save(*out, c); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "[analyze] %d notes, %.1f BPM, %.1fs → %s\n",
		len(c.Notes), c.BPM, c.Duration, *out)
}
