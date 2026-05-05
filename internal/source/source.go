// Package source resolves an input string (local path or URL) to a local audio file path.
// URLs (YouTube, etc.) are downloaded via yt-dlp into a cache dir.
package source

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsURL returns true if input looks like an http(s) URL.
func IsURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// CacheDir returns ~/.cache/tempo-tty (creating it lazily on first download).
func CacheDir() string {
	if x := os.Getenv("XDG_CACHE_HOME"); x != "" {
		return filepath.Join(x, "tempo-tty")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "tempo-tty")
}

// Resolve returns a usable local audio path. If progress != nil, it is called
// with status strings during download (so caller can render a spinner/log).
func Resolve(input string, progress func(string)) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty source")
	}
	if !IsURL(input) {
		abs, err := filepath.Abs(input)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("file not found: %s", abs)
		}
		return abs, nil
	}
	return downloadYouTube(input, progress)
}

func downloadYouTube(url string, progress func(string)) (string, error) {
	if _, err := exec.LookPath("yt-dlp"); err != nil {
		return "", fmt.Errorf("yt-dlp not installed (brew install yt-dlp)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return "", fmt.Errorf("ffmpeg not installed (brew install ffmpeg)")
	}

	dir := CacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	report := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	report("resolving video id…")

	idOut, err := exec.Command("yt-dlp", "--no-playlist", "--get-id", url).Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp get-id: %w", err)
	}
	id := strings.TrimSpace(string(idOut))
	if id == "" {
		return "", fmt.Errorf("could not extract video id from %s", url)
	}

	// cache hit?
	if matches, _ := filepath.Glob(filepath.Join(dir, id+".*")); len(matches) > 0 {
		for _, m := range matches {
			ext := strings.ToLower(filepath.Ext(m))
			if ext == ".mp3" || ext == ".wav" || ext == ".ogg" || ext == ".flac" {
				report("using cached " + filepath.Base(m))
				return m, nil
			}
		}
	}

	report("downloading audio…")
	tmpl := filepath.Join(dir, "%(id)s.%(ext)s")
	cmd := exec.Command(
		"yt-dlp",
		"--no-playlist",
		"--no-progress",
		"-x", "--audio-format", "mp3",
		"-o", tmpl,
		url,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("yt-dlp download failed: %w\n%s", err, string(out))
	}

	matches, _ := filepath.Glob(filepath.Join(dir, id+".mp3"))
	if len(matches) == 0 {
		// fallback: any extension
		matches, _ = filepath.Glob(filepath.Join(dir, id+".*"))
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("download finished but no output file in %s", dir)
	}
	report("downloaded " + filepath.Base(matches[0]))
	return matches[0], nil
}
