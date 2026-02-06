package webui

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"md2slack/internal/gitdiff"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Settings struct {
	ProjectPaths []string `json:"project_paths"`
	Usernames    []string `json:"usernames"`
}

type ProjectInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".md2slack", "webui.json"), nil
}

func loadSettings(path string) (Settings, error) {
	if strings.TrimSpace(path) == "" {
		p, err := settingsPath()
		if err != nil {
			return Settings{}, err
		}
		path = p
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Settings{}, nil
		}
		return Settings{}, err
	}

	var s Settings
	if err := json.Unmarshal(b, &s); err != nil {
		return Settings{}, err
	}
	return normalizeSettings(s), nil
}

func saveSettings(path string, s Settings) error {
	if strings.TrimSpace(path) == "" {
		p, err := settingsPath()
		if err != nil {
			return err
		}
		path = p
	}

	s = normalizeSettings(s)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func normalizeSettings(s Settings) Settings {
	s.ProjectPaths = normalizeList(s.ProjectPaths)
	s.Usernames = normalizeList(s.Usernames)
	return s
}

func normalizeList(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, v := range values {
		clean := strings.TrimSpace(v)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	sort.Strings(out)
	return out
}

func buildProjectInfo(paths []string) []ProjectInfo {
	var projects []ProjectInfo
	for _, p := range normalizeList(paths) {
		name := gitdiff.GetRepoNameAt(p)
		projects = append(projects, ProjectInfo{Name: name, Path: p})
	}
	return projects
}

func scanUsers(repoPath string) ([]string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repo path is required")
	}
	nameOut, _ := runGitInRepo(repoPath, "git config user.name")
	logOut, _ := runGitInRepo(repoPath, "git log --format=%an -n 200")
	var users []string
	if strings.TrimSpace(nameOut) != "" {
		users = append(users, strings.TrimSpace(nameOut))
	}
	for _, line := range strings.Split(logOut, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		users = append(users, strings.TrimSpace(line))
	}
	return normalizeList(users), nil
}

func runGitInRepo(repoPath string, cmd string) (string, error) {
	c := exec.Command("bash", "-c", cmd)
	c.Dir = repoPath
	var out bytes.Buffer
	var errOut bytes.Buffer
	c.Stdout = &out
	c.Stderr = &errOut
	if err := c.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git command failed: %s", msg)
	}
	return strings.TrimSpace(out.String()), nil
}
