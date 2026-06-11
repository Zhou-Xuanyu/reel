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
	SampleRate        int     // e.g. 44100
	ChannelLayout     string  // e.g. "stereo"
	Codec             string  // e.g. "aac"
	Bitrate           string  // e.g. "192k"
	NormalizeLoudness bool    // run loudnorm per input to equalize volume across clips
	TargetLUFS        float64 // target integrated loudness when NormalizeLoudness is true (e.g. -16)
	Fade              bool    // apply fade-in and fade-out per clip
	FadeSeconds       float64 // duration of each fade in seconds (e.g. 0.15)
}

// runFFmpeg invokes ffmpeg to concatenate files into out using the concat
// filter. Each input is normalized to s.SampleRate + s.ChannelLayout
// (optionally loudness-normalized via loudnorm), then concatenated and
// re-encoded with s.Codec at s.Bitrate.
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
// input audio stream and concatenates them.
//
// Per-input normalization chain (without loudnorm or fade):
//
//	[N:a] aresample=44100, aformat=channel_layouts=stereo [aN]
//
// With loudnorm enabled:
//
//	[N:a] aresample=44100, aformat=channel_layouts=stereo,
//	      loudnorm=I=-16:TP=-1.5:LRA=11 [aN]
//
// With fade enabled, fade-in + tail fade-out are appended. The tail fade
// uses areverse,afade=t=in,areverse so we don't need each input's duration:
//
//	[N:a] aresample=..., aformat=...,
//	      afade=t=in:d=0.15,
//	      areverse, afade=t=in:d=0.15, areverse [aN]
//
// Then all normalized labels feed into the concat filter:
//
//	[a0][a1]...[aN-1] concat=n=N:v=0:a=1 [out]
//
// loudnorm params:
//   - I  = target integrated loudness in LUFS (s.TargetLUFS). -16 = podcast,
//     -14 = YouTube/Spotify, -23 = broadcast.
//   - TP = true peak ceiling in dBTP. -1.5 leaves headroom to avoid clipping
//     after lossy re-encode.
//   - LRA = loudness range. 11 LU is typical.
//
// fade params:
//   - d  = fade duration in seconds (s.FadeSeconds). 0.15 is gentle.
func buildConcatFilter(n int, s ffmpegSettings) string {
	normChain := fmt.Sprintf("aresample=%d,aformat=channel_layouts=%s", s.SampleRate, s.ChannelLayout)
	if s.NormalizeLoudness {
		normChain += fmt.Sprintf(",loudnorm=I=%.1f:TP=-1.5:LRA=11", s.TargetLUFS)
	}
	if s.Fade {
		normChain += fmt.Sprintf(",afade=t=in:d=%.3f,areverse,afade=t=in:d=%.3f,areverse",
			s.FadeSeconds, s.FadeSeconds)
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
