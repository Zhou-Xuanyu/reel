package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sortSource names a time-resolution strategy. Config.SortBy is a list of
// these strings; resolveTime tries each in order until one returns a time.
type sortSource string

const (
	sortMetadata sortSource = "metadata"
	sortFilename sortSource = "filename"
	sortMtime    sortSource = "mtime"
)

// clip pairs a file path with the time we resolved for sorting.
type clip struct {
	Path   string
	Time   time.Time
	Source sortSource
}

// collect returns absolute paths of every audio file directly inside dir,
// sorted by the configured sort chain. Files whose resolvers all return zero
// are appended after the resolved set in alphabetical order.
func collect(dir string, allowedExts, sortChain []string) ([]string, error) {
	allowed := make(map[string]bool, len(allowedExts))
	for _, ext := range allowedExts {
		allowed[strings.ToLower(ext)] = true
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var resolved []clip
	var unresolved []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if !allowed[ext] {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		t, src, ok := resolveTime(abs, sortChain)
		if !ok {
			unresolved = append(unresolved, abs)
			continue
		}
		resolved = append(resolved, clip{Path: abs, Time: t, Source: src})
	}

	if len(resolved)+len(unresolved) == 0 {
		return nil, fmt.Errorf("no audio files found in %s", dir)
	}

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].Time.Before(resolved[j].Time)
	})
	sort.Strings(unresolved)

	fmt.Printf("found %d audio files in %s\n", len(resolved)+len(unresolved), dir)
	for _, c := range resolved {
		fmt.Printf("  %s [%s] %s\n",
			c.Time.Local().Format(time.RFC3339), c.Source, filepath.Base(c.Path))
	}
	for _, p := range unresolved {
		fmt.Printf("  ?                          [unresolved] %s\n", filepath.Base(p))
	}

	files := make([]string, 0, len(resolved)+len(unresolved))
	for _, c := range resolved {
		files = append(files, c.Path)
	}
	files = append(files, unresolved...)
	return files, nil
}

// resolveTime walks the chain. Returns the first non-zero time and the
// source that produced it. ok=false if every strategy returned zero.
func resolveTime(path string, chain []string) (time.Time, sortSource, bool) {
	for _, key := range chain {
		var t time.Time
		switch sortSource(key) {
		case sortMetadata:
			t, _ = timeFromMetadata(path)
		case sortFilename:
			t, _ = timeFromFilename(path)
		case sortMtime:
			t, _ = timeFromMtime(path)
		}
		if !t.IsZero() {
			return t, sortSource(key), true
		}
	}
	return time.Time{}, "", false
}

// timeFromMetadata reads format.tags.creation_time via ffprobe.
// Returns zero time (no error) if the tag is missing or unparseable.
func timeFromMetadata(path string) (time.Time, error) {
	r, err := probeFile(path)
	if err != nil {
		return time.Time{}, err
	}
	raw := r.Format.Tags["creation_time"]
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		// fall back to plain RFC3339 (no fractional seconds)
		t, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, nil
		}
	}
	return t, nil
}

// filenameDatePatterns: tried in order; first match wins. Each pattern
// captures (year, month, day, hour, minute, second).
var filenameDatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(\d{4})(\d{2})(\d{2})[_T](\d{2})(\d{2})(\d{2})`),         // 20240115_143022
	regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})[_T](\d{2})-(\d{2})-(\d{2})`),     // 2024-01-15_14-30-22
	regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})\s(\d{2})\.(\d{2})\.(\d{2})`),     // 2024-01-15 14.30.22
}

// timeFromFilename matches common embedded date patterns in the basename.
// Returns zero time (no error) if no pattern matches.
func timeFromFilename(path string) (time.Time, error) {
	base := filepath.Base(path)
	for _, re := range filenameDatePatterns {
		m := re.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		h, _ := strconv.Atoi(m[4])
		mi, _ := strconv.Atoi(m[5])
		s, _ := strconv.Atoi(m[6])
		return time.Date(y, time.Month(mo), d, h, mi, s, 0, time.Local), nil
	}
	return time.Time{}, nil
}

// timeFromMtime returns the filesystem modification time. Least reliable —
// copying or syncing a file updates this. True birthtime (creation) is
// macOS-specific and would need syscall.Stat_t.Birthtimespec.
func timeFromMtime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
