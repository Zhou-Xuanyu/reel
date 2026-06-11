package main

import (
	"fmt"
	"os"
)

const usage = `reel — toolkit for merging voice memo recordings

Commands:
  cp     copy voice memos out of the Apple Voice Memos library by date range
  ls     generate a merge playlist from a folder of clips (and optional transitions)
  merge  read a playlist file and concatenate it with ffmpeg

Run "reel <command> -h" for command-specific help.

Typical pipeline:
  reel cp --from=2026-06-08
  reel ls --transition=transitions
  reel merge
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "cp":
		runCp(args)
	case "ls":
		runLs(args)
	case "merge":
		runMerge(args)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

func die(args ...any) {
	fmt.Fprintln(os.Stderr, append([]any{"error:"}, args...)...)
	os.Exit(1)
}
