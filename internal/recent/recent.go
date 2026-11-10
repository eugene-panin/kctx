package recent

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

const maxRecent = 10

// Dir returns XDG state dir for kctx (~/.local/state/kctx).
func Dir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "kctx")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "kctx")
}

func recentPath() string    { return filepath.Join(Dir(), "recent") }
func favoritesPath() string { return filepath.Join(Dir(), "favorites") }

// Touch records name as most-recently used.
func Touch(name string) error {
	if name == "" {
		return nil
	}
	list, _ := List()
	out := []string{name}
	for _, n := range list {
		if n != name {
			out = append(out, n)
		}
		if len(out) >= maxRecent {
			break
		}
	}
	return writeLines(recentPath(), out)
}

// List returns recent context names (most recent first).
func List() ([]string, error) {
	return readLines(recentPath())
}

// Favorites returns pinned context names.
func Favorites() ([]string, error) {
	return readLines(favoritesPath())
}

// ToggleFavorite adds or removes name from favorites. Returns whether it is now favorited.
func ToggleFavorite(name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	list, _ := Favorites()
	for i, n := range list {
		if n == name {
			list = append(list[:i], list[i+1:]...)
			return false, writeLines(favoritesPath(), list)
		}
	}
	list = append(list, name)
	return true, writeLines(favoritesPath(), list)
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			out = append(out, line)
		}
	}
	return out, sc.Err()
}

func writeLines(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
