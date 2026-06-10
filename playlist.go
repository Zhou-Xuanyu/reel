package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// collect returns absolute paths of every audio file directly inside dir,
// sorted alphabetically by filename. For Apple Voice Memos with their native
// `YYYYMMDD HHMMSS` naming, alphabetical = chronological. User-renamed
// exports sort by their custom name (original record time is unrecoverable).
func collect(dir string, allowedExts []string) ([]string, error) {
	files, err := listAudioFiles(dir, allowedExts)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no audio files found in %s", dir)
	}
	fmt.Printf("found %d audio files in %s\n", len(files), dir)
	for _, f := range files {
		fmt.Printf("  %s\n", filepath.Base(f))
	}
	return files, nil
}

// listAudioFiles returns absolute paths of audio files directly inside dir,
// filtered by extension allowlist and sorted alphabetically. Quiet helper
// shared by collect and the transition source.
func listAudioFiles(dir string, allowedExts []string) ([]string, error) {
	allowed := make(map[string]bool, len(allowedExts))
	for _, ext := range allowedExts {
		allowed[strings.ToLower(ext)] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !allowed[ext] {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, abs)
	}
	sort.Strings(files)
	return files, nil
}
