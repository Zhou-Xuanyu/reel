package audio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// audioExts is the set of file extensions treated as audio (case-insensitive
// match via ToLower at the call site).
var audioExts = map[string]bool{
	".m4a":  true,
	".mp3":  true,
	".wav":  true,
	".flac": true,
	".ogg":  true,
	".aac":  true,
}

// Collect returns absolute paths of every audio file directly inside dir,
// sorted alphabetically. Errors if the directory is unreadable or empty
// of audio files.
func Collect(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !audioExts[ext] {
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
