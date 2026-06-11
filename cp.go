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

// voiceMemoDate matches Apple's Voice Memos filename prefix: 8 digits, space,
// 6 digits. Example: "20240115 143022-ABCD1234.m4a".
var voiceMemoDate = regexp.MustCompile(`^(\d{8} \d{6})`)

// voiceMemoExts is the set of audio extensions Voice Memos produces. Older
// recordings use .m4a; newer iOS uses .qta. Both are AAC under the hood.
var voiceMemoExts = map[string]bool{
	".m4a": true,
	".qta": true,
}

// runCp copies (or symlinks) Apple Voice Memos out of the system library
// into a destination folder, filtered by a date range.
func runCp(args []string) {
	defaultSrc := filepath.Join(os.Getenv("HOME"),
		"Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings")

	fs := flag.NewFlagSet("reel cp", flag.ExitOnError)
	src := fs.String("src", defaultSrc, "source folder (Voice Memos library)")
	dst := fs.String("to-dir", "./voice-memo", "destination folder")
	from := fs.String("from", "", "inclusive start date YYYY-MM-DD (required)")
	to := fs.String("to", "", "inclusive end date YYYY-MM-DD (default: today)")
	hardCopy := fs.Bool("copy", false, "hard copy instead of symlink (default: symlink)")
	byDate := fs.Bool("by-date", false, "group copies into per-date subfolders under to-dir")
	dryRun := fs.Bool("dry-run", false, "list matches without copying or linking")
	fs.Parse(args)

	// validate the date range as wall-clock labels (YYYYMMDD strings)
	fromDate, toDate := parseDateRange(*from, *to)

	// scan src for Voice Memos whose filename date falls inside [from, to]
	matches := findInRange(*src, fromDate, toDate)

	// always print what we found before any disk action
	printMatches(matches, fromDate, toDate)

	// preview mode stops here — nothing touches dst
	if *dryRun {
		return
	}

	// symlink or hard-copy each match into dst (optionally grouped by date)
	transferAll(matches, *src, *dst, *hardCopy, *byDate)
}

// parseDateRange validates --from (required) and --to (default today) as
// YYYY-MM-DD wall-clock date labels and returns them in YYYYMMDD form for
// string comparison against filename prefixes.
//
// Voice Memos filenames are pure wall-clock labels — they carry no timezone
// information. We compare them as text, never as absolute instants. The
// local clock is only consulted to *read* what date the user means by
// "today" when --to is omitted; the result is then just another label.
func parseDateRange(fromStr, toStr string) (string, string) {
	if fromStr == "" {
		die("reel cp: --from is required (YYYY-MM-DD)")
	}
	fromKey := mustNormalizeDate(fromStr, "--from")

	if toStr == "" {
		// today as the local clock displays it
		toStr = time.Now().Format("2006-01-02")
	}
	toKey := mustNormalizeDate(toStr, "--to")

	if fromKey > toKey {
		die("--from is after --to")
	}
	return fromKey, toKey
}

// mustNormalizeDate validates a YYYY-MM-DD string and returns its YYYYMMDD
// form (no separators) for direct string comparison.
func mustNormalizeDate(s, name string) string {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		die("parse "+name+":", err)
	}
	return strings.ReplaceAll(s, "-", "")
}

// formatDateKey turns YYYYMMDD back into YYYY-MM-DD for display.
func formatDateKey(key string) string {
	return key[:4] + "-" + key[4:6] + "-" + key[6:8]
}

// findInRange scans src for Voice Memos in the given date range. Dies on
// scan error or no matches.
func findInRange(src, fromDate, toDate string) []voiceMemo {
	matches, err := findVoiceMemos(src, fromDate, toDate)
	if err != nil {
		die(err)
	}
	if len(matches) == 0 {
		die(fmt.Sprintf("no voice memos in %s..%s",
			formatDateKey(fromDate), formatDateKey(toDate)))
	}
	return matches
}

// printMatches prints a header line and one row per match showing its
// recorded time + filename.
func printMatches(matches []voiceMemo, fromDate, toDate string) {
	fmt.Printf("found %d voice memos in %s..%s\n",
		len(matches), formatDateKey(fromDate), formatDateKey(toDate))
	for _, m := range matches {
		fmt.Printf("  %s  %s\n", m.at.Format("2006-01-02 15:04:05"), m.name)
	}
}

// transferAll creates dstDir (and per-date subdirs if byDate is on), then
// symlinks (or hard copies) each match. Dies on the first failure. Prints
// a final summary line.
func transferAll(matches []voiceMemo, srcDir, dstDir string, hardCopy, byDate bool) {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		die("mkdir destination:", err)
	}
	for _, m := range matches {
		targetDir := dstDir
		if byDate {
			targetDir = filepath.Join(dstDir, m.at.Format("2006-01-02"))
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				die("mkdir "+targetDir+":", err)
			}
		}
		srcPath := filepath.Join(srcDir, m.name)
		dstPath := filepath.Join(targetDir, m.name)
		if err := transfer(srcPath, dstPath, hardCopy); err != nil {
			die("transfer "+m.name+":", err)
		}
	}
	action := "linked"
	if hardCopy {
		action = "copied"
	}
	layout := dstDir
	if byDate {
		layout = dstDir + "/<YYYY-MM-DD>/"
	}
	fmt.Printf("%s %d files -> %s\n", action, len(matches), layout)
}

// voiceMemo pairs a file's basename with the time.Time form of its filename
// digits. The time.Time is used for display and for the by-date subfolder
// name only — never compared for filtering. Date filtering is done by
// string comparison against the YYYYMMDD prefix elsewhere.
type voiceMemo struct {
	name string
	at   time.Time
}

// findVoiceMemos lists Voice Memos in src whose filename date prefix
// (YYYYMMDD) falls inside the inclusive [fromDate, toDate] range. Sorted
// by filename, which is chronological because Voice Memos names start
// with `YYYYMMDD HHMMSS`.
func findVoiceMemos(src, fromDate, toDate string) ([]voiceMemo, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s\n", src)
		}
		return nil, fmt.Errorf("read src: %w", err)
	}

	var out []voiceMemo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !voiceMemoExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		m := voiceMemoDate.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		// pure string filter on the YYYYMMDD portion
		dateKey := m[1][:8]
		if dateKey < fromDate || dateKey > toDate {
			continue
		}
		// parse for display / subfolder name only; not used for comparison
		at, err := time.Parse("20060102 150405", m[1])
		if err != nil {
			continue
		}
		out = append(out, voiceMemo{name: e.Name(), at: at})
	}

	// Sort by filename. Voice Memos names start with `YYYYMMDD HHMMSS`,
	// so lexicographic order is chronological.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].name < out[j-1].name; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// transfer creates dst as either a symlink to src (default) or a hard byte
// copy. Overwrites any existing dst.
func transfer(src, dst string, hardCopy bool) error {
	srcAbs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	os.Remove(dst) // ignored; let create surface real failures

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

	_, err = io.Copy(out, in)
	return err
}
