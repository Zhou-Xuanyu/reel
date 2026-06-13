package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// runMerge reads a playlist file (or all .reel playlists under a directory
// when --recursive is set) and merges each into one output via ffmpeg.
func runMerge(args []string) {
	fs := flag.NewFlagSet("reel merge", flag.ExitOnError)
	playlist := fs.String("playlist", "output/voice-memo.reel", "playlist file path (- for stdin); with --recursive, a directory to walk")
	out := fs.String("out", "", "output file path (default: sibling .m4a next to playlist); ignored with --recursive")
	recursive := fs.Bool("recursive", false, "walk --playlist as a directory for .reel files; merge each into a sibling .m4a")
	sampleRate := fs.Int("sample-rate", 44100, "target sample rate (Hz)")
	channelLayout := fs.String("channel-layout", "stereo", "target channel layout (mono/stereo)")
	codec := fs.String("codec", "aac", "output codec (aac/libmp3lame/flac/pcm_s16le/...)")
	bitrate := fs.String("bitrate", "192k", "output bitrate")
	normalize := fs.Bool("normalize", true, "loudness-normalize each input (loudnorm)")
	lufs := fs.Float64("lufs", -16, "target integrated loudness in LUFS")
	dryRun := fs.Bool("dry-run", false, "preview: list input counts and total duration; skip ffmpeg")
	fs.Parse(args)

	// pack flags into encode + normalization settings
	settings := ffmpegSettings{
		SampleRate:        *sampleRate,
		ChannelLayout:     *channelLayout,
		Codec:             *codec,
		Bitrate:           *bitrate,
		NormalizeLoudness: *normalize,
		TargetLUFS:        *lufs,
	}

	if *recursive {
		// with --recursive, fall back to the output root when --playlist
		// wasn't set explicitly (its default is a file, not a folder).
		dir := *playlist
		if !flagWasSet(fs, "playlist") {
			dir = "output"
		}
		if *dryRun {
			dryRunRecursive(dir)
			return
		}
		mergeAllRecursive(dir, settings)
		return
	}

	// single-playlist mode
	if *dryRun {
		dryRunSingle(*playlist)
		return
	}
	files := loadPlaylist(*playlist)
	outPath := resolveOutPath(*playlist, *out)
	concatTo(outPath, files, settings)
}

// dryRunSingle reports inputs + projected duration for one playlist.
func dryRunSingle(path string) {
	files, err := readPlaylist(path)
	if err != nil {
		die(err)
	}
	if len(files) == 0 {
		die("playlist is empty:", path)
	}
	reportPlaylistStats(path, files)
}

// dryRunRecursive walks dir for *.reel and reports per-playlist + grand
// totals.
func dryRunRecursive(dir string) {
	info, err := os.Stat(dir)
	if err != nil {
		die("read "+dir+":", err)
	}
	if !info.IsDir() {
		die("--recursive requires --playlist to be a directory; got", dir)
	}

	playlists := findReelPlaylists(dir)
	if len(playlists) == 0 {
		die("no .reel playlists found under", dir)
	}
	fmt.Printf("found %d playlists under %s\n", len(playlists), dir)

	var grandFiles int
	var grandSec float64
	for _, p := range playlists {
		files, err := readPlaylist(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warn: %s: %v\n", p, err)
			continue
		}
		sec := reportPlaylistStats(p, files)
		grandFiles += len(files)
		grandSec += sec
	}
	fmt.Printf("grand total: %d inputs, ~%s\n", grandFiles, formatDuration(grandSec))
}

// reportPlaylistStats probes each file for duration, prints a summary line,
// and returns the summed duration in seconds.
func reportPlaylistStats(path string, files []string) float64 {
	var total float64
	failed := 0
	for _, f := range files {
		d, err := probeDuration(f)
		if err != nil {
			failed++
			continue
		}
		total += d
	}
	if failed > 0 {
		fmt.Printf("%s: %d inputs, ~%s (%d probe failures)\n",
			path, len(files), formatDuration(total), failed)
	} else {
		fmt.Printf("%s: %d inputs, ~%s\n", path, len(files), formatDuration(total))
	}
	return total
}

// probeDuration returns the duration in seconds of a media file via ffprobe.
func probeDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// formatDuration prints a number of seconds as e.g. "2h13m5s".
func formatDuration(sec float64) string {
	total := int(sec + 0.5)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// resolveOutPath returns the explicit --out value, or a sibling .m4a path
// next to the playlist file. Stdin / blank basename falls back to
// output/merged.m4a.
func resolveOutPath(playlist, override string) string {
	if override != "" {
		return override
	}
	base := strings.TrimSuffix(filepath.Base(playlist), filepath.Ext(playlist))
	if base == "" || base == "-" {
		return "output/merged.m4a"
	}
	return filepath.Join(filepath.Dir(playlist), base+".m4a")
}

// mergeAllRecursive walks dir for *.reel files and merges each into a
// sibling .m4a. Dies on bad path or no playlists found.
func mergeAllRecursive(dir string, s ffmpegSettings) {
	info, err := os.Stat(dir)
	if err != nil {
		die("read "+dir+":", err)
	}
	if !info.IsDir() {
		die("--recursive requires --playlist to be a directory; got", dir)
	}

	playlists := findReelPlaylists(dir)
	if len(playlists) == 0 {
		die("no .reel playlists found under", dir)
	}
	fmt.Printf("found %d playlists under %s\n", len(playlists), dir)

	for _, p := range playlists {
		outPath := strings.TrimSuffix(p, filepath.Ext(p)) + ".m4a"
		files := loadPlaylist(p)
		concatTo(outPath, files, s)
	}
}

// findReelPlaylists walks dir for *.reel files, sorted by full path.
func findReelPlaylists(dir string) []string {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".reel") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		die("walk "+dir+":", err)
	}
	sort.Strings(out)
	return out
}

// flagWasSet reports whether the named flag was supplied by the user (as
// opposed to taking its default value).
func flagWasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// loadPlaylist reads paths from the playlist file (or stdin), prints a
// count, and dies on error or empty result.
func loadPlaylist(path string) []string {
	files, err := readPlaylist(path)
	if err != nil {
		die(err)
	}
	if len(files) == 0 {
		die("playlist is empty:", path)
	}
	fmt.Printf("read %d files from %s\n", len(files), path)
	return files
}

// concatTo invokes ffmpeg to merge files into out. Creates the parent
// directory if needed. Dies on ffmpeg failure.
func concatTo(out string, files []string, s ffmpegSettings) {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		die("mkdir output dir:", err)
	}
	if err := runFFmpeg(files, out, s); err != nil {
		die(err)
	}
}

// readPlaylist parses a playlist file: one absolute path per line. Lines
// that are empty or start with `#` are ignored. Pass "-" to read from stdin.
func readPlaylist(path string) ([]string, error) {
	var src *os.File
	if path == "-" {
		src = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open playlist: %w", err)
		}
		defer f.Close()
		src = f
	}

	var files []string
	scanner := bufio.NewScanner(src)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		files = append(files, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read playlist: %w", err)
	}
	return files, nil
}

// ffmpegSettings holds the encode + normalization knobs for a merge.
type ffmpegSettings struct {
	SampleRate        int
	ChannelLayout     string
	Codec             string
	Bitrate           string
	NormalizeLoudness bool
	TargetLUFS        float64
}

// runFFmpeg concatenates files into out using ffmpeg's concat filter.
// Each input is normalized (resample + channel layout, optional loudnorm)
// then joined into one stream. The output's title metadata is set to the
// basename of `out` (e.g. "voice-memo" or "2026-06-08") so players don't
// show the first input's leaked tag (e.g. "New Recording 26").
//
// Built command for N inputs:
//
//	ffmpeg -y \
//	  -i in0 -i in1 ... -i inN-1 \
//	  -filter_complex "<normalize each + concat -> [out]>" \
//	  -map [out] \
//	  -c:a <codec> -b:a <bitrate> \
//	  -map_metadata -1 -metadata title=<basename> \
//	  out
func runFFmpeg(files []string, out string, s ffmpegSettings) error {
	title := strings.TrimSuffix(filepath.Base(out), filepath.Ext(out))

	args := []string{"-y"}
	for _, f := range files {
		args = append(args, "-i", f)
	}
	args = append(args,
		"-filter_complex", buildConcatFilter(len(files), s),
		"-map", "[out]",
		"-c:a", s.Codec,
		"-b:a", s.Bitrate,
		"-map_metadata", "-1", // drop any tags inherited from inputs
		"-metadata", "title="+title,
		out,
	)

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Printf("merged -> %s\n", out)
	return nil
}

// buildConcatFilter returns a -filter_complex string that normalizes each
// input audio stream and concatenates them. For n=3, no loudnorm:
//
//	[0:a]aresample=44100,aformat=channel_layouts=stereo[a0];
//	[1:a]aresample=44100,aformat=channel_layouts=stereo[a1];
//	[2:a]aresample=44100,aformat=channel_layouts=stereo[a2];
//	[a0][a1][a2]concat=n=3:v=0:a=1[out]
//
// With loudnorm, each per-input chain also includes:
//
//	,loudnorm=I=-16:TP=-1.5:LRA=11
func buildConcatFilter(n int, s ffmpegSettings) string {
	normChain := fmt.Sprintf("aresample=%d,aformat=channel_layouts=%s",
		s.SampleRate, s.ChannelLayout)
	if s.NormalizeLoudness {
		normChain += fmt.Sprintf(",loudnorm=I=%.1f:TP=-1.5:LRA=11", s.TargetLUFS)
	}

	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, "[%d:a]%s[a%d];", i, normChain, i)
	}
	for i := range n {
		fmt.Fprintf(&b, "[a%d]", i)
	}
	fmt.Fprintf(&b, "concat=n=%d:v=0:a=1[out]", n)
	return b.String()
}
