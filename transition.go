package main

import (
	"fmt"
	"math/rand/v2"
)

// transitionSource yields a transition audio file path on each call to next().
// All transitions come from a user-supplied folder; insertion is either
// random or sequential (cyclic) per cfg.TransitionRandom.
type transitionSource struct {
	pool    []string
	random  bool
	counter int // sequential mode: next index is counter % len(pool)
}

// newTransitionSource loads audio files from cfg.TransitionPath into a pool.
// Errors if the folder is missing or contains no audio files — same policy
// as the input dir.
func newTransitionSource(cfg Config) (*transitionSource, error) {
	pool, err := listAudioFiles(cfg.TransitionPath, cfg.AudioExts)
	if err != nil {
		return nil, fmt.Errorf("transition folder %s: %w", cfg.TransitionPath, err)
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("no audio files in transition folder %s", cfg.TransitionPath)
	}
	return &transitionSource{
		pool:   pool,
		random: cfg.TransitionRandom,
	}, nil
}

// describe returns a short human-readable summary of what next() will yield.
func (t *transitionSource) describe() string {
	mode := "sequential"
	if t.random {
		mode = "random"
	}
	return fmt.Sprintf("%s from %d files", mode, len(t.pool))
}

// next returns the path for the next transition insertion.
// Random mode: uniform pick. Sequential mode: cycle through the sorted pool.
func (t *transitionSource) next() string {
	if t.random {
		return t.pool[rand.IntN(len(t.pool))]
	}
	p := t.pool[t.counter%len(t.pool)]
	t.counter++
	return p
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
