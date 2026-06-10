package main

import (
	"fmt"
	"os"
	"reel/internal/audio"
)

type Config struct {
	Dir    string
	Output string
}

func defaultConfig() Config {
	return Config{
		Dir:    "voice-memo",
		Output: "merged.m4a",
	}
}

func main() {
	// load config
	cfg := defaultConfig()

	// collect files
	files, err := audio.Collect(cfg.Dir)
	if err != nil {
		die(err)
	}

	// run ffmpeg
	if err := audio.RunFFmpeg(files, cfg.Output); err != nil {
		die(err)
	}
}

func die(args ...any) {
	fmt.Fprintln(os.Stderr, append([]any{"error:"}, args...)...)
	os.Exit(1)
}
