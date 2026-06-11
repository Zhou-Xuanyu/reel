package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// audioExts is the extension allowlist for the playlist input scan.
var audioExts = map[string]bool{
	".m4a":  true,
	".mp3":  true,
	".wav":  true,
	".flac": true,
	".ogg":  true,
	".aac":  true,
	".qta":  true,
}

// runLs generates a merge playlist from a folder of clips, optionally
// interleaving transition cues from a second folder. Three modes:
//
//   - flat       : direct files in --dir, single playlist (default).
//   - recursive  : walk all of --dir, single playlist.
//   - per-folder : one playlist per direct subfolder of --dir; each
//     collected recursively within its subfolder.
func runLs(args []string) {
	fs := flag.NewFlagSet("reel ls", flag.ExitOnError)
	dir := fs.String("dir", "voice-memo", "input folder of audio clips")
	trans := fs.String("transition", "", "folder of transition cues; empty = no transitions")
	random := fs.Bool("random", true, "randomize transition order (false = sequential cycle)")
	mode := fs.String("mode", "flat", "scan mode: flat | recursive | per-folder")
	out := fs.String("out", "", "output path; default depends on mode (- for stdout, single-playlist modes only)")
	fs.Parse(args)

	// transition pool (shared across all playlists when in per-folder mode)
	transitions := loadTransitions(*trans)

	// dispatch by mode
	switch *mode {
	case "flat":
		emitFlat(*dir, transitions, *trans, *random, *out)
	case "recursive":
		emitRecursive(*dir, transitions, *trans, *random, *out)
	case "per-folder":
		emitPerFolder(*dir, transitions, *trans, *random, *out)
	default:
		die("invalid --mode:", *mode, "(want flat | recursive | per-folder)")
	}
}

// emitFlat: direct files in dir → one playlist.
func emitFlat(dir string, transitions []string, transDir string, random bool, out string) {
	clips := loadClips(dir)
	files := interleave(clips, transitions, random)

	if out == "" {
		out = filepath.Join("output", filepath.Base(dir)+".txt")
	}
	writePlaylistTo(out, files, dir, transDir, random, len(clips))
}

// emitRecursive: walk all of dir → one playlist.
func emitRecursive(dir string, transitions []string, transDir string, random bool, out string) {
	clips := loadClipsRecursive(dir)
	files := interleave(clips, transitions, random)

	if out == "" {
		out = filepath.Join("output", filepath.Base(dir)+".txt")
	}
	writePlaylistTo(out, files, dir, transDir, random, len(clips))
}

// emitPerFolder: one playlist per direct subfolder of dir; each subfolder
// is scanned recursively for audio.
func emitPerFolder(dir string, transitions []string, transDir string, random bool, outDir string) {
	if outDir == "" {
		outDir = filepath.Join("output", filepath.Base(dir))
	}
	subs := listSubfolders(dir)
	if len(subs) == 0 {
		die("no subfolders in", dir, "(per-folder mode requires subfolders)")
	}

	written := 0
	for _, sub := range subs {
		subPath := filepath.Join(dir, sub)
		clips, err := walkAudio(subPath)
		if err != nil {
			die("walk "+subPath+":", err)
		}
		if len(clips) == 0 {
			continue
		}
		files := interleave(clips, transitions, random)
		outPath := filepath.Join(outDir, sub+".txt")
		writePlaylistTo(outPath, files, subPath, transDir, random, len(clips))
		written++
	}
	if written == 0 {
		die("no audio files found in any subfolder of", dir)
	}
}

// loadClips reads audio files from dir (one level) and dies on error or
// empty result.
func loadClips(dir string) []string {
	files, err := listAudio(dir)
	if err != nil {
		die(err)
	}
	if len(files) == 0 {
		die("no audio files in", dir)
	}
	return files
}

// loadClipsRecursive walks dir recursively and dies on error or empty.
func loadClipsRecursive(dir string) []string {
	files, err := walkAudio(dir)
	if err != nil {
		die(err)
	}
	if len(files) == 0 {
		die("no audio files under", dir, "(recursive)")
	}
	return files
}

// loadTransitions reads audio files from dir when dir is non-empty. Returns
// nil when dir is empty (transitions disabled). Dies on error or empty dir.
func loadTransitions(dir string) []string {
	if dir == "" {
		return nil
	}
	files, err := listAudio(dir)
	if err != nil {
		die("transitions:", err)
	}
	if len(files) == 0 {
		die("no audio files in transition folder", dir)
	}
	return files
}

// writePlaylistTo writes the playlist to path (or stdout if path == "-").
// Creates the parent directory if needed. Prints a summary line on success
// when writing to a file.
func writePlaylistTo(path string, files []string, dir, trans string, random bool, clipCount int) {
	if path == "-" {
		if err := writePlaylist(os.Stdout, files, dir, trans, random); err != nil {
			die("write playlist:", err)
		}
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		die("mkdir output dir:", err)
	}
	f, err := os.Create(path)
	if err != nil {
		die("create playlist:", err)
	}
	defer f.Close()

	if err := writePlaylist(f, files, dir, trans, random); err != nil {
		die("write playlist:", err)
	}
	fmt.Printf("wrote playlist with %d entries (%d clips, %d transitions) -> %s\n",
		len(files), clipCount, len(files)-clipCount, path)
}

// listAudio returns absolute paths of audio files directly inside dir
// (non-recursive), filtered by audioExts and sorted alphabetically.
func listAudio(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !audioExts[strings.ToLower(filepath.Ext(e.Name()))] {
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

// walkAudio returns absolute paths of audio files anywhere under dir
// (recursive), filtered by audioExts and sorted alphabetically.
func walkAudio(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !audioExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return absErr
		}
		files = append(files, abs)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// listSubfolders returns the names of dir's immediate subdirectories,
// sorted alphabetically. Empty slice if dir has no subdirs.
func listSubfolders(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		die("read "+dir+":", err)
	}
	var subs []string
	for _, e := range entries {
		if e.IsDir() {
			subs = append(subs, e.Name())
		}
	}
	sort.Strings(subs)
	return subs
}

// interleave returns clips with a transition inserted between each pair.
// random=true picks each from transitions uniformly; random=false cycles
// through transitions in order. Returns clips unchanged if transitions are
// empty or there's only one clip.
func interleave(clips, transitions []string, random bool) []string {
	if len(clips) <= 1 || len(transitions) == 0 {
		return clips
	}
	out := make([]string, 0, len(clips)*2-1)
	out = append(out, clips[0])
	counter := 0
	for _, c := range clips[1:] {
		var t string
		if random {
			t = transitions[rand.IntN(len(transitions))]
		} else {
			t = transitions[counter%len(transitions)]
			counter++
		}
		out = append(out, t, c)
	}
	return out
}

// writePlaylist writes a header + one path per line. Header lines start
// with `#`; readers skip them.
func writePlaylist(w io.Writer, files []string, dir, trans string, random bool) error {
	fmt.Fprintf(w, "# generated by reel ls at %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "# dir: %s\n", dir)
	if trans != "" {
		mode := "sequential"
		if random {
			mode = "random"
		}
		fmt.Fprintf(w, "# transition: %s (%s)\n", trans, mode)
	}
	fmt.Fprintf(w, "# entries: %d\n\n", len(files))
	for _, f := range files {
		if _, err := fmt.Fprintln(w, f); err != nil {
			return err
		}
	}
	return nil
}
