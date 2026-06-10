package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// collect returns absolute paths of every audio file directly inside dir,
// sorted alphabetically. allowedExts filters by extension; comparison is
// case-insensitive against the lowercased filename extension.
func collect(dir string, allowedExts []string) ([]string, error) {
	allowed := make(map[string]bool, len(allowedExts))
	for _, ext := range allowedExts {
		allowed[strings.ToLower(ext)] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
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

	if len(files) == 0 {
		return nil, fmt.Errorf("no audio files found in %s", dir)
	}
	fmt.Printf("found %d audio files in %s\n", len(files), dir)
	return files, nil
}
