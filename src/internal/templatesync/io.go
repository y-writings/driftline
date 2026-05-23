package templatesync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func loadManifest(path string) (Manifest, error) {
	var manifest Manifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, fmt.Errorf("read manifest: %w", err)
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return manifest, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.Version != 1 {
		return manifest, fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	seen := map[string]struct{}{}
	for _, item := range manifest.Template {
		if item.ID == "" {
			return manifest, errors.New("manifest contains template without id")
		}
		if item.Source == "" || item.Target == "" {
			return manifest, fmt.Errorf("template %q must define source and target", item.ID)
		}
		if _, ok := seen[item.ID]; ok {
			return manifest, fmt.Errorf("duplicate template id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return manifest, nil
}

func loadLock(path string) (LockFile, error) {
	lock := LockFile{Files: map[string]LockItem{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return lock, nil
	}
	if err != nil {
		return lock, fmt.Errorf("read lock file: %w", err)
	}
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return lock, fmt.Errorf("parse lock file: %w", err)
	}
	if lock.Files == nil {
		lock.Files = map[string]LockItem{}
	}
	return lock, nil
}

func WriteLock(path string, lock LockFile) error {
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode lock file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lock file directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

func fileHash(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), true, nil
}

func CopyFile(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func pathWithin(root, name, label string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("%s path must be relative: %s", label, name)
	}
	clean := filepath.Clean(name)
	if clean == "." {
		return "", fmt.Errorf("%s path must refer to a file: %s", label, name)
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%s path escapes root: %s", label, name)
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve %s root: %w", label, err)
	}
	fullPath := filepath.Join(rootAbs, clean)
	rel, err := filepath.Rel(rootAbs, fullPath)
	if err != nil {
		return "", fmt.Errorf("resolve %s path: %w", label, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%s path escapes root: %s", label, name)
	}
	return fullPath, nil
}


func EnsureGitIgnore(path string, entries []string) error {
	if len(entries) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	content := string(data)
	existing := map[string]struct{}{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		existing[line] = struct{}{}
	}
	missing := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		if _, ok := existing[trimmed]; ok {
			continue
		}
		missing = append(missing, trimmed)
		existing[trimmed] = struct{}{}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf := content
	if buf != "" && !strings.HasSuffix(buf, "\n") {
		buf += "\n"
	}
	buf += strings.Join(missing, "\n") + "\n"
	return os.WriteFile(path, []byte(buf), 0o644)
}
