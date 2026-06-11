package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runMerge reads a playlist file and merges every listed audio file into
// one output via ffmpeg.
func runMerge(args []string) {
	fs := flag.NewFlagSet("reel merge", flag.ExitOnError)
	playlist := fs.String("playlist", "playlist.txt", "playlist file path (- for stdin)")
	out := fs.String("out", "merged.m4a", "output file path")
	sampleRate := fs.Int("sample-rate", 44100, "target sample rate (Hz)")
	channelLayout := fs.String("channel-layout", "stereo", "target channel layout (mono/stereo)")
	codec := fs.String("codec", "aac", "output codec (aac/libmp3lame/flac/pcm_s16le/...)")
	bitrate := fs.String("bitrate", "192k", "output bitrate")
	normalize := fs.Bool("normalize", true, "loudness-normalize each input (loudnorm)")
	lufs := fs.Float64("lufs", -16, "target integrated loudness in LUFS")
	fs.Parse(args)

	// read absolute input paths from the playlist file
	files := loadPlaylist(*playlist)

	// pack flags into encode + normalization settings
	settings := ffmpegSettings{
		SampleRate:        *sampleRate,
		ChannelLayout:     *channelLayout,
		Codec:             *codec,
		Bitrate:           *bitrate,
		NormalizeLoudness: *normalize,
		TargetLUFS:        *lufs,
	}

	// concat + encode via ffmpeg
	concatTo(*out, files, settings)
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

// concatTo invokes ffmpeg to merge files into out. Dies on ffmpeg failure.
func concatTo(out string, files []string, s ffmpegSettings) {
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
// then joined into one stream.
//
// Built command for N inputs:
//
//	ffmpeg -y \
//	  -i in0 -i in1 ... -i inN-1 \
//	  -filter_complex "<normalize each + concat -> [out]>" \
//	  -map [out] \
//	  -c:a <codec> -b:a <bitrate> \
//	  out
func runFFmpeg(files []string, out string, s ffmpegSettings) error {
	args := []string{"-y"}
	for _, f := range files {
		args = append(args, "-i", f)
	}
	args = append(args,
		"-filter_complex", buildConcatFilter(len(files), s),
		"-map", "[out]",
		"-c:a", s.Codec,
		"-b:a", s.Bitrate,
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
