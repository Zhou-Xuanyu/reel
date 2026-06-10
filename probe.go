package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// probeResult mirrors the relevant subset of `ffprobe -print_format json
// -show_format`. We only need format.tags right now; extend as needed.
type probeResult struct {
	Format struct {
		Tags map[string]string `json:"tags"`
	} `json:"format"`
}

// probeFile runs ffprobe on path and parses the JSON output.
func probeFile(path string) (*probeResult, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_format",
		"-print_format", "json",
		path,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	var r probeResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return &r, nil
}
