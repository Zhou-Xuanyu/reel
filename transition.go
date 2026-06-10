package main

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"os"
)

// beepBytes is the pre-recorded short tone used when the user enables
// transitions without specifying a path. Generated once via:
//
//	ffmpeg -y -f lavfi -i "sine=frequency=800:duration=0.5" \
//	  -af "afade=t=in:d=0.05,afade=t=out:st=0.45:d=0.05" \
//	  -c:a aac -b:a 192k assets/beep.m4a
//
//go:embed assets/beep.m4a
var beepBytes []byte

// transitionSource yields a transition audio file path on each call to next().
// Either fixed (same path every time) or pool (random pick from list).
type transitionSource struct {
	fixed   string   // single-file mode: returned every call
	pool    []string // random mode: sampled each call
	cleanup func()   // tear down temp resources (e.g. delete written beep)
}

// newTransitionSource resolves cfg.TransitionPath into a source. Behavior
// depends on what the path points at:
//
//   - "" (empty)      → use the bundled beep, write to a temp file once
//   - regular file    → use that file directly
//   - directory       → list audio inside, random pick each call
//
// The caller must invoke (*transitionSource).cleanup() when the merge is done.
func newTransitionSource(cfg Config) (*transitionSource, error) {
	if cfg.TransitionPath == "" {
		tempPath, err := writeBeepTemp()
		if err != nil {
			return nil, fmt.Errorf("write beep: %w", err)
		}
		return &transitionSource{
			fixed:   tempPath,
			cleanup: func() { os.Remove(tempPath) },
		}, nil
	}

	info, err := os.Stat(cfg.TransitionPath)
	if err != nil {
		return nil, fmt.Errorf("transition path: %w", err)
	}

	if info.IsDir() {
		pool, err := listAudioFiles(cfg.TransitionPath, cfg.AudioExts)
		if err != nil {
			return nil, fmt.Errorf("list transition folder: %w", err)
		}
		if len(pool) == 0 {
			return nil, fmt.Errorf("no audio files in transition folder %s", cfg.TransitionPath)
		}
		return &transitionSource{
			pool:    pool,
			cleanup: func() {},
		}, nil
	}

	return &transitionSource{
		fixed:   cfg.TransitionPath,
		cleanup: func() {},
	}, nil
}

// describe returns a short human-readable summary of what next() will yield.
func (t *transitionSource) describe() string {
	if len(t.pool) > 0 {
		return fmt.Sprintf("random from %d files", len(t.pool))
	}
	return t.fixed
}

// next returns the path for the next transition insertion.
func (t *transitionSource) next() string {
	if len(t.pool) > 0 {
		return t.pool[rand.IntN(len(t.pool))]
	}
	return t.fixed
}

// interleave returns clips with the transition source's next() inserted
// between each consecutive pair. For [a, b, c]: [a, t1, b, t2, c].
// Returns clips unchanged if length <= 1.
func interleave(clips []string, ts *transitionSource) []string {
	if len(clips) <= 1 {
		return clips
	}
	out := make([]string, 0, len(clips)*2-1)
	out = append(out, clips[0])
	for _, c := range clips[1:] {
		out = append(out, ts.next(), c)
	}
	return out
}

// writeBeepTemp materializes the embedded beep bytes to a temp file so ffmpeg
// can read it as an input. Returns the temp file path; caller deletes when
// done. No runtime synthesis — bytes are baked in at compile time.
func writeBeepTemp() (string, error) {
	f, err := os.CreateTemp("", "reel-beep-*.m4a")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(beepBytes); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}
