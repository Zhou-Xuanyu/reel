# Reel

CLI that merges a folder of audio recordings (primarily Apple Voice Memos) into one continuous file. Built around ffmpeg's concat filter — handles mixed input formats, normalizes loudness so clips sound equally loud, and optionally inserts transition cues between clips.

## Requirements

- **Go 1.22+** (uses `for i := range n` and `math/rand/v2`)
- **ffmpeg** on `$PATH` — install via `brew install ffmpeg` (macOS) or your package manager

## Install / build

```
git clone <repo>
cd reel
go build -o reel       # produces ./reel
```

Or run without building:

```
go run .
```

## Quick start

```
mkdir voice-memo
cp ~/your-recordings/*.m4a voice-memo/
./reel
```

Reads every audio file in `voice-memo/`, sorts alphabetically, normalizes loudness, merges to `merged.m4a` in the current directory.

## Configuration

Three layers, lowest to highest priority:

1. **Built-in defaults** (in `defaultConfig()`)
2. **`reel.json`** in the current directory (optional)
3. **CLI flags**

Each layer overrides only the fields it sets. Missing keys in `reel.json` keep the default.

### Config fields

| JSON key | Flag | Type | Default | Meaning |
|---|---|---|---|---|
| `dir` | `--dir` | string | `voice-memo` | Input folder (non-recursive). |
| `output` | `--out` | string | `merged.m4a` | Output file path. Extension picks the container. |
| `audio_exts` | — | string[] | `.m4a .mp3 .wav .flac .ogg .aac` | Extensions treated as audio (case-insensitive). |
| `sample_rate` | `--sample-rate` | int | `44100` | Target sample rate in Hz. All inputs resampled to this. |
| `channel_layout` | `--channel-layout` | string | `stereo` | Target channel layout (`mono`/`stereo`). |
| `audio_codec` | `--codec` | string | `aac` | Output codec (`aac`/`libmp3lame`/`flac`/`pcm_s16le`/...). |
| `audio_bitrate` | `--bitrate` | string | `192k` | Output bitrate (e.g. `192k`, `256k`). |
| `normalize_loudness` | `--normalize` | bool | `true` | Run `loudnorm` per input so clips land at the same loudness. |
| `target_lufs` | `--lufs` | float | `-16` | Target integrated loudness. `-16` podcast, `-14` streaming, `-23` broadcast. |
| `transition` | `--transition` | bool | `false` | Insert transitions between clips. |
| `transition_path` | `--transition-path` | string | `transitions` | Folder containing transition audio files. |
| `transition_random` | `--transition-random` | bool | `true` | Random pick vs sequential cycle through the folder. |

### Example `reel.json`

```json
{
  "dir": "my-recordings",
  "output": "out.m4a",
  "audio_bitrate": "256k",
  "normalize_loudness": true,
  "target_lufs": -14,
  "transition": true,
  "transition_path": "cues",
  "transition_random": false
}
```

### Flag examples

```
./reel                                       # all defaults
./reel --dir=podcasts --out=show.mp3 --codec=libmp3lame
./reel --lufs=-14                            # louder target
./reel --normalize=false                     # skip loudness pass
./reel --transition                          # interleave cues
./reel --transition --transition-random=false
./reel --transition --transition-path=stings
```

CLI flags **override** anything in `reel.json`.

## Output format

The output container is chosen from the extension of `--out` / `output`. The codec is set by `--codec` / `audio_codec`. They must be compatible:

| Output extension | Use codec |
|---|---|
| `.m4a`, `.mp4` | `aac` |
| `.mp3` | `libmp3lame` |
| `.flac` | `flac` |
| `.wav` | `pcm_s16le` (raw 16-bit PCM) |
| `.ogg` | `libvorbis` or `libopus` |

Mismatch (e.g. `--out=x.wav --codec=aac`) → ffmpeg errors.

## How sorting works

Files in the input directory are sorted **alphabetically by filename**. For Apple Voice Memos with their native `YYYYMMDD HHMMSS` naming, alphabetical = chronological. For user-renamed exports, sort order follows the custom name (the original record time is unrecoverable from the file alone).

Apple's Voice Memos may live in `~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/`.

## Transitions

When `--transition` is on, reel reads audio files from `--transition-path` (default `transitions/`) and inserts one between each consecutive pair of clips.

- **Random mode** (default): uniform random pick from the folder for every gap. Repeats allowed.
- **Sequential mode** (`--transition-random=false`): cycle through the folder in sorted order. Pool of 3, 5 gaps → `t0, t1, t2, t0, t1`.

If the folder is missing or contains no audio files, reel errors out (same policy as the input dir).

## Loudness normalization

Voice Memos recorded under different conditions sound very different. By default reel runs ffmpeg's `loudnorm` filter on each input before concat. Each clip is shifted toward the target LUFS, so all clips sound equally loud in the output.

- `-16 LUFS` (default) — typical podcast / spoken word.
- `-14 LUFS` — Spotify, YouTube, Apple Music streaming targets.
- `-23 LUFS` — EBU R128 broadcast standard.

Disable with `--normalize=false` if you want raw level preserved. Transitions also pass through normalization when enabled — a sine-wave beep at −16 LUFS is loud; either use a quieter cue file or disable normalization for runs with transitions.

## Pipeline overview

```
┌─────────────┐
│ voice-memo/ │
└──────┬──────┘
       │
       ▼
┌──────────────────────────────────────────┐
│ playlist.go · collect()                  │
│ filter by extension, sort by filename    │
└──────┬───────────────────────────────────┘
       │  []string (voice memo paths)
       ▼
┌──────────────────────────────────────────┐
│ transition.go · interleave()  [optional] │
│ insert cues between consecutive clips    │
└──────┬───────────────────────────────────┘
       │  []string (final input list)
       ▼
┌──────────────────────────────────────────┐
│ ffmpeg.go · runFFmpeg()                  │
│ build -filter_complex:                   │
│   [N:a] aresample, aformat, loudnorm     │
│   concat=n=N → [out]                     │
│ spawn ffmpeg subprocess                  │
└──────┬───────────────────────────────────┘
       │
       ▼
┌─────────────┐
│ merged.m4a  │
└─────────────┘
```

## Files in this repo

| File | Role |
|---|---|
| `main.go` | Entry point. Resolves config, calls `collect` → `interleave` → `runFFmpeg`. |
| `config.go` | `Config` struct, defaults, JSON loader, flag parser. |
| `playlist.go` | `collect()` and `listAudioFiles()` — read input directory. |
| `transition.go` | `transitionSource`, random/sequential picker, `interleave()`. |
| `ffmpeg.go` | `ffmpegSettings`, `runFFmpeg()`, filter graph builder. |
| `voice-memo/` | Default input directory. |
| `transitions/` | Default transition cue directory. |

## Todo

1. release
1. tool for selecting Apple's voice memo by time period
1. manual or help page
