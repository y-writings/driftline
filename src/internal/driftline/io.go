package driftline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadPullConfig(path string) (PullConfig, error) {
	var cfg PullConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read pull config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse pull config: %w", err)
	}
	for i := range cfg.Pull {
		if cfg.Pull[i].Repo == "" {
			return cfg, fmt.Errorf("pull[%d] repo is required", i)
		}
	}
	return cfg, nil
}

func loadExportConfig(path string) (ExportConfig, error) {
	var raw map[string]any
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read export config: %w", err)
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse export config: %w", err)
	}
	out := ExportConfig{}
	for key, value := range raw {
		paths, err := flattenExportEntries(value, "")
		if err != nil {
			return nil, fmt.Errorf("export %q: %w", key, err)
		}
		out[key] = paths
	}
	return out, nil
}

func flattenExportEntries(v any, prefix string) ([]string, error) {
	switch t := v.(type) {
	case []any:
		var out []string
		for _, child := range t {
			items, err := flattenExportEntries(child, prefix)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
		}
		return out, nil
	case map[string]any:
		var out []string
		for dir, child := range t {
			next := filepath.Join(prefix, dir)
			items, err := flattenExportEntries(child, next)
			if err != nil {
				return nil, err
			}
			out = append(out, items...)
		}
		return out, nil
	case string:
		return []string{filepath.Join(prefix, t)}, nil
	default:
		return nil, fmt.Errorf("unsupported export entry type %T", v)
	}
}

func loadLock(path string) (LockFile, error) {
	var lock LockFile
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return lock, nil
		}
		return lock, fmt.Errorf("read lock file: %w", err)
	}
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return lock, fmt.Errorf("parse lock file: %w", err)
	}
	return lock, nil
}

func WriteLock(path string, lock LockFile) error {
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode lock file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

func mapRepoToGitURL(repo string) string {
	if strings.HasPrefix(repo, "github:") {
		return "https://github.com/" + strings.TrimPrefix(repo, "github:") + ".git"
	}
	if strings.HasPrefix(repo, ":github:") {
		return "https://github.com/" + strings.TrimPrefix(repo, ":github:") + ".git"
	}
	return repo
}


func LoadPullConfigPublic(path string) (PullConfig, error) { return loadPullConfig(path) }
func LoadExportConfigPublic(path string) (ExportConfig, error) { return loadExportConfig(path) }
func LoadLockPublic(path string) (LockFile, error) { return loadLock(path) }
func MapRepoToGitURLPublic(repo string) string { return mapRepoToGitURL(repo) }
