// play is a thin CLI wrapper around internal/game for power users.
// For the integrated TUI use cmd/tempo-tty.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/gdamore/tcell/v2"

	"tempo-tty/internal/chart"
	"tempo-tty/internal/game"
)

func main() {
	keys := flag.String("keys", "dfjk", "lane→key mapping")
	speed := flag.Float64("speed", 3.5, "seconds for note to travel right→judgement line")
	offset := flag.Float64("offset", 0.0, "timing offset (sec); +adjusts later, -adjusts earlier")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: play [flags] <chart.json>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(2)
	}

	c, err := chart.Load(flag.Arg(0))
	if err != nil {
		fail(err)
	}

	scr, err := tcell.NewScreen()
	if err != nil {
		fail(err)
	}
	if err := scr.Init(); err != nil {
		fail(err)
	}
	scr.HideCursor()

	res, err := game.Run(scr, c, game.Options{
		Speed: *speed, Offset: *offset, Keys: *keys,
	})
	scr.Fini()
	if err != nil {
		fail(err)
	}

	fmt.Println()
	fmt.Println("=== Result ===")
	fmt.Printf("Score:      %d\n", res.Score)
	fmt.Printf("Max Combo:  %d\n", res.MaxCombo)
	for _, k := range []string{"PERFECT", "GREAT", "GOOD", "MISS"} {
		fmt.Printf("  %-8s %d\n", k, res.Counts[k])
	}
	fmt.Printf("Accuracy:   %.2f%%\n", res.Accuracy)
	if res.NewOffset != *offset {
		fmt.Printf("Offset adjusted live to: %+.3fs (use -offset %.3f next time)\n",
			res.NewOffset, res.NewOffset)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
