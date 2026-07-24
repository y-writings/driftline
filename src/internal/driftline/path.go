package driftline

import (
	"path/filepath"
	"strings"
)

func normalizedConfigPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func isPathAncestor(parent string, child string) bool {
	parent = normalizedConfigPath(parent)
	child = normalizedConfigPath(child)
	return parent != child && strings.HasPrefix(child, parent+"/")
}
