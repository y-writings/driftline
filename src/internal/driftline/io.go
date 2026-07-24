package driftline

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fileHash(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return hashBytes(data), true, nil
}

func writeFileBytes(target string, data []byte) error {
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
