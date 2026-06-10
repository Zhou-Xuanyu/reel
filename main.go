package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

func main() {
	// resolve config from defaults + file + flags
	cfg, err := resolveConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		die(err)
	}

	// collect input files
	files, err := collect(cfg.Dir, cfg.AudioExts)
	if err != nil {
		die(err)
	}

	// run ffmpeg
	if err := runFFmpeg(files, cfg.Output, cfg.settings()); err != nil {
		die(err)
	}
}

func die(args ...any) {
	fmt.Fprintln(os.Stderr, append([]any{"error:"}, args...)...)
	os.Exit(1)
}
