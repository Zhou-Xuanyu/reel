package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

const defaultConfigPath = "reel.json"

type Config struct {
	Dir               string   `json:"dir"`
	Output            string   `json:"output"`
	AudioExts         []string `json:"audio_exts"`
	SampleRate        int      `json:"sample_rate"`
	ChannelLayout     string   `json:"channel_layout"`
	AudioCodec        string   `json:"audio_codec"`
	AudioBitrate      string   `json:"audio_bitrate"`
	NormalizeLoudness bool     `json:"normalize_loudness"`
	TargetLUFS        float64  `json:"target_lufs"`
	Fade              bool     `json:"fade"`
	FadeSeconds       float64  `json:"fade_seconds"`
	Transition        bool     `json:"transition"`
	TransitionPath    string   `json:"transition_path"`
	TransitionRandom  bool     `json:"transition_random"`
}

func defaultConfig() Config {
	return Config{
		Dir:               "voice-memo",
		Output:            "merged.m4a",
		AudioExts:         []string{".m4a", ".mp3", ".wav", ".flac", ".ogg", ".aac", ".qta"},
		SampleRate:        44100,
		ChannelLayout:     "stereo",
		AudioCodec:        "aac",
		AudioBitrate:      "192k",
		NormalizeLoudness: true,
		TargetLUFS:        -16,
		Fade:              false,
		FadeSeconds:       0.15,
		Transition:        false,
		TransitionPath:    "transitions",
		TransitionRandom:  true,
	}
}

// resolveConfig builds the final Config from layered sources, lowest to
// highest priority:
//  1. defaultConfig() — baseline values.
//  2. JSON file at defaultConfigPath, if it exists.
//  3. CLI flags from args.
//
// Each layer only overrides fields it sets explicitly.
func resolveConfig(args []string) (Config, error) {
	cfg := defaultConfig()

	if _, err := os.Stat(defaultConfigPath); err == nil {
		loaded, err := loadFromFile(cfg, defaultConfigPath)
		if err != nil {
			return cfg, err
		}
		cfg = loaded
		fmt.Printf("loaded config from %s\n", defaultConfigPath)
	}

	return applyFlags(cfg, args)
}

// loadFromFile reads JSON at path and overlays it on cfg via json.Unmarshal.
// Fields missing from the JSON keep their existing values.
func loadFromFile(cfg Config, path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// applyFlags parses args, overlaying any provided CLI flag on cfg. Each flag
// defaults to its current cfg value, so unset flags leave cfg untouched.
func applyFlags(cfg Config, args []string) (Config, error) {
	fs := flag.NewFlagSet("reel", flag.ContinueOnError)
	fs.StringVar(&cfg.Dir, "dir", cfg.Dir, "input directory")
	fs.StringVar(&cfg.Output, "out", cfg.Output, "output file path")
	fs.IntVar(&cfg.SampleRate, "sample-rate", cfg.SampleRate, "target sample rate (Hz)")
	fs.StringVar(&cfg.ChannelLayout, "channel-layout", cfg.ChannelLayout, "target channel layout (mono/stereo)")
	fs.StringVar(&cfg.AudioCodec, "codec", cfg.AudioCodec, "output audio codec (aac/libmp3lame/flac/...)")
	fs.StringVar(&cfg.AudioBitrate, "bitrate", cfg.AudioBitrate, "output audio bitrate (e.g. 192k)")
	fs.BoolVar(&cfg.NormalizeLoudness, "normalize", cfg.NormalizeLoudness, "normalize each input's loudness (loudnorm) so clips sound equally loud")
	fs.Float64Var(&cfg.TargetLUFS, "lufs", cfg.TargetLUFS, "target integrated loudness in LUFS (e.g. -16 podcast, -14 streaming, -23 broadcast)")
	fs.BoolVar(&cfg.Fade, "fade", cfg.Fade, "apply fade-in and fade-out to each clip (avoids click pops at boundaries)")
	fs.Float64Var(&cfg.FadeSeconds, "fade-seconds", cfg.FadeSeconds, "duration of each fade in seconds (e.g. 0.15)")
	fs.BoolVar(&cfg.Transition, "transition", cfg.Transition, "insert transitions between clips (requires audio files in the transition folder)")
	fs.StringVar(&cfg.TransitionPath, "transition-path", cfg.TransitionPath, "folder containing transition audio files")
	fs.BoolVar(&cfg.TransitionRandom, "transition-random", cfg.TransitionRandom, "randomize transition order (false = sequential cycle)")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// settings extracts ffmpeg-related fields into a small struct so runFFmpeg's
// signature stays narrow.
func (c Config) settings() ffmpegSettings {
	return ffmpegSettings{
		SampleRate:        c.SampleRate,
		ChannelLayout:     c.ChannelLayout,
		Codec:             c.AudioCodec,
		Bitrate:           c.AudioBitrate,
		NormalizeLoudness: c.NormalizeLoudness,
		TargetLUFS:        c.TargetLUFS,
		Fade:              c.Fade,
		FadeSeconds:       c.FadeSeconds,
	}
}
