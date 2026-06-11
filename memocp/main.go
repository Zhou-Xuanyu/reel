// memocp copies (or symlinks) Apple Voice Memo recordings out of the system
// Voice Memos library into a destination folder, filtered by an inclusive
// date range derived from each file's `YYYYMMDD HHMMSS` filename prefix.
//
// Usage:
//
//	memocp --from=2024-01-01 --to=2024-03-31
//	memocp --from=2024-01-01 --copy           # hard copy instead of symlink
//	memocp --from=2024-01-01 --dry-run        # preview matches only
//
// Requires Full Disk Access for the source folder under modern macOS.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// dateRegex matches Apple's Voice Memos filename prefix: 8 digits, a space,
// then 6 digits. Example: "20240115 143022-ABCD1234.m4a".
var dateRegex = regexp.MustCompile(`^(\d{8} \d{6})`)

// voiceMemoExts is the set of audio extensions Voice Memos produces. Older
// recordings use .m4a; newer iOS versions use .qta (Apple's compressed
// container). Both are AAC at the codec level.
var voiceMemoExts = map[string]bool{
	".m4a": true,
	".qta": true,
}

func main() {
	defaultSrc := filepath.Join(os.Getenv("HOME"),
		"Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings")

	src := flag.String("src", defaultSrc, "source folder (Voice Memos library)")
	dst := flag.String("to-dir", "./voice-memo", "destination folder")
	from := flag.String("from", "", "inclusive start date YYYY-MM-DD (required)")
	to := flag.String("to", "", "inclusive end date YYYY-MM-DD (default: today)")
	hardCopy := flag.Bool("copy", false, "hard copy instead of symlink (default: symlink)")
	dryRun := flag.Bool("dry-run", false, "print matches without copying or linking")
	flag.Parse()

	if *from == "" {
		die("--from is required (YYYY-MM-DD)")
	}
	fromTime, err := time.ParseInLocation("2006-01-02", *from, time.Local)
	if err != nil {
		die("parse --from:", err)
	}

	toTime := time.Now()
	if *to != "" {
		t, err := time.ParseInLocation("2006-01-02", *to, time.Local)
		if err != nil {
			die("parse --to:", err)
		}
		// include the whole end day
		toTime = t.Add(24*time.Hour - time.Second)
	}
	if fromTime.After(toTime) {
		die("--from is after --to")
	}

	matches, err := findMatches(*src, fromTime, toTime)
	if err != nil {
		die(err)
	}
	if len(matches) == 0 {
		die(fmt.Sprintf("no voice memos in %s..%s",
			fromTime.Format("2006-01-02"), toTime.Format("2006-01-02")))
	}

	fmt.Printf("found %d voice memos in %s..%s\n",
		len(matches), fromTime.Format("2006-01-02"), toTime.Format("2006-01-02"))
	for _, m := range matches {
		fmt.Printf("  %s  %s\n", m.at.Format("2006-01-02 15:04:05"), m.name)
	}

	if *dryRun {
		return
	}

	if err := os.MkdirAll(*dst, 0o755); err != nil {
		die("mkdir destination:", err)
	}

	for _, m := range matches {
		srcPath := filepath.Join(*src, m.name)
		dstPath := filepath.Join(*dst, m.name)
		if err := transfer(srcPath, dstPath, *hardCopy); err != nil {
			die("transfer", m.name+":", err)
		}
	}

	action := "linked"
	if *hardCopy {
		action = "copied"
	}
	fmt.Printf("%s %d files -> %s\n", action, len(matches), *dst)
}

// match pairs a file's basename with its parsed record time.
type match struct {
	name string
	at   time.Time
}

// findMatches lists .m4a files in src whose filename date falls inside the
// inclusive range [from, to]. Sorted by date.
func findMatches(src string, from, to time.Time) ([]match, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s\n  Grant Full Disk Access to your terminal:\n  System Settings → Privacy & Security → Full Disk Access", src)
		}
		return nil, fmt.Errorf("read src: %w", err)
	}

	var out []match
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !voiceMemoExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		at, ok := parseFilenameDate(e.Name())
		if !ok {
			continue
		}
		if at.Before(from) || at.After(to) {
			continue
		}
		out = append(out, match{name: e.Name(), at: at})
	}

	// Sort by recorded time ascending.
	sortByTime(out)
	return out, nil
}

// parseFilenameDate extracts the leading `YYYYMMDD HHMMSS` from a filename.
// Returns the local-time instant + true on success.
func parseFilenameDate(name string) (time.Time, bool) {
	m := dateRegex.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation("20060102 150405", m[1], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// sortByTime sorts matches in place, ascending by time.
func sortByTime(ms []match) {
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j].at.Before(ms[j-1].at); j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
}

// transfer creates dst as either a symlink to src (default) or a hard byte
// copy of src. Overwrites any existing dst.
func transfer(src, dst string, hardCopy bool) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	os.Remove(dst) // ignore error; allow create to surface real failures

	if !hardCopy {
		return os.Symlink(srcAbs, dst)
	}

	in, err := os.Open(srcAbs)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func die(args ...any) {
	fmt.Fprintln(os.Stderr, append([]any{"error:"}, args...)...)
	os.Exit(1)
}
