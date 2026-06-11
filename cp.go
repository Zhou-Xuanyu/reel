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
	dryRun := fs.Bool("dry-run", false, "list matches without copying or linking")
	fs.Parse(args)

	if *from == "" {
		die("reel cp: --from is required (YYYY-MM-DD)")
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
		// include the full end day
		toTime = t.Add(24*time.Hour - time.Second)
	}
	if fromTime.After(toTime) {
		die("--from is after --to")
	}

	matches, err := findVoiceMemos(*src, fromTime, toTime)
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
			die("transfer "+m.name+":", err)
		}
	}
	action := "linked"
	if *hardCopy {
		action = "copied"
	}
	fmt.Printf("%s %d files -> %s\n", action, len(matches), *dst)
}

// voiceMemo pairs a file's basename with its parsed record time.
type voiceMemo struct {
	name string
	at   time.Time
}

// findVoiceMemos lists Voice Memos in src whose filename-derived date falls
// inside the inclusive range [from, to], sorted by date.
func findVoiceMemos(src string, from, to time.Time) ([]voiceMemo, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s\n  Grant Full Disk Access to your terminal:\n  System Settings → Privacy & Security → Full Disk Access", src)
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
		at, err := time.ParseInLocation("20060102 150405", m[1], time.Local)
		if err != nil {
			continue
		}
		if at.Before(from) || at.After(to) {
			continue
		}
		out = append(out, voiceMemo{name: e.Name(), at: at})
	}

	// Ascending by recorded time.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].at.Before(out[j-1].at); j-- {
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
