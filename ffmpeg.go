package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ffmpegSettings holds ffmpeg encode + normalization knobs. Populated by the
// caller from Config; no defaults live here.
type ffmpegSettings struct {
	SampleRate    int    // e.g. 44100
	ChannelLayout string // e.g. "stereo"
	Codec         string // e.g. "aac"
	Bitrate       string // e.g. "192k"
}

// runFFmpeg invokes ffmpeg to concatenate files into out using the concat
// filter. Each input is normalized to s.SampleRate + s.ChannelLayout, then
// concatenated and re-encoded with s.Codec at s.Bitrate.
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
		"-filter_complex", buildConcatFilter(len(files), s.SampleRate, s.ChannelLayout),
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
// input audio stream and concatenates them. For n=3 it produces:
//
//	[0:a]aresample=44100,aformat=channel_layouts=stereo[a0];
//	[1:a]aresample=44100,aformat=channel_layouts=stereo[a1];
//	[2:a]aresample=44100,aformat=channel_layouts=stereo[a2];
//	[a0][a1][a2]concat=n=3:v=0:a=1[out]
//
// Filter graph syntax cheat sheet:
//
//   - [N:a]            select stream N's audio track as filter input
//   - aresample=R      resample to rate R Hz (matches all inputs)
//   - aformat=...      force a channel layout (e.g. stereo)
//   - [aN]             label this normalized stream so concat can reference it
//   - concat=n=N:v=0:a=1  join N inputs; v=0 audio-only; a=1 emit one audio stream
//   - [out]            label the final stream so -map [out] can pick it up
//   - `;` separates filter chains, `,` chains filters in one chain
func buildConcatFilter(n, sampleRate int, channelLayout string) string {
	var b strings.Builder

	for i := range n {
		fmt.Fprintf(&b, "[%d:a]aresample=%d,aformat=channel_layouts=%s[a%d];",
			i, sampleRate, channelLayout, i)
	}

	for i := range n {
		fmt.Fprintf(&b, "[a%d]", i)
	}

	fmt.Fprintf(&b, "concat=n=%d:v=0:a=1[out]", n)
	return b.String()
}
