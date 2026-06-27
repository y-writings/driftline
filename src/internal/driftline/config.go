package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	SourceManifestPath = ".driftline-source.toml"
	TargetConfigPath   = ".driftline-target.toml"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func LoadSourceManifestBytes(data []byte) (SourceManifest, error) {
	var manifest SourceManifest
	metadata, err := toml.Decode(string(data), &manifest)
	if err != nil {
		return manifest, fmt.Errorf("parse source manifest: %w", err)
	}
	if err := rejectUndecoded("source manifest", metadata.Undecoded()); err != nil {
		return manifest, err
	}
	return manifest, validateSourceManifest(manifest)
}

func LoadTargetConfig(path string) (TargetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetConfig{}, fmt.Errorf("read target config: %w", err)
	}
	return LoadTargetConfigBytes(data)
}

func LoadTargetConfigBytes(data []byte) (TargetConfig, error) {
	var config TargetConfig
	metadata, err := toml.Decode(string(data), &config)
	if err != nil {
		return config, fmt.Errorf("parse target config: %w", err)
	}
	if err := rejectUndecoded("target config", metadata.Undecoded()); err != nil {
		return config, err
	}
	return config, validateTargetConfig(config)
}

func WriteTargetConfig(path string, config TargetConfig) error {
	commit, cleanup, err := PrepareTargetConfigWrite(path, config)
	if err != nil {
		return err
	}
	defer cleanup()
	return commit()
}

func PrepareTargetConfigWrite(path string, config TargetConfig) (func() error, func() error, error) {
	if err := validateTargetConfig(config); err != nil {
		return nil, nil, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("create target config directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".driftline-target-*.toml")
	if err != nil {
		return nil, nil, fmt.Errorf("create target config temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() error {
		err := os.Remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := temp.WriteString(FormatTargetConfig(config)); err != nil {
		temp.Close()
		cleanup()
		return nil, nil, fmt.Errorf("write target config temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close target config temp file: %w", err)
	}
	commit := func() error {
		return os.Rename(tempName, path)
	}
	return commit, cleanup, nil
}

func FormatTargetConfig(config TargetConfig) string {
	var b strings.Builder
	b.WriteString("version = 2\n\n")
	b.WriteString("[source]\n")
	b.WriteString("repository = ")
	b.WriteString(strconv.Quote(config.Source.Repository))
	b.WriteString("\n")
	b.WriteString("ref = ")
	b.WriteString(strconv.Quote(config.Source.Ref))
	b.WriteString("\n")

	for _, group := range sortedStringKeys(config.Files) {
		files := config.Files[group]
		if len(files) == 0 {
			continue
		}
		b.WriteString("\n[files.")
		b.WriteString(group)
		b.WriteString("]\n")
		for _, file := range sortedStringKeys(files) {
			b.WriteString(file)
			b.WriteString(" = ")
			b.WriteString(strconv.Quote(files[file]))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TargetConfigFromSourceManifest(repository string, ref string, manifest SourceManifest) (TargetConfig, error) {
	if err := ValidateRepository(repository); err != nil {
		return TargetConfig{}, err
	}
	if err := ValidateRef(ref); err != nil {
		return TargetConfig{}, err
	}
	config := TargetConfig{
		Version: 2,
		Source: TargetSource{
			Repository: repository,
			Ref:        ref,
		},
		Files: map[string]map[string]string{},
	}
	for _, entry := range SourceEntries(manifest) {
		if entry.Mode != ModeManaged {
			continue
		}
		ensureTargetGroup(config.Files, entry.Group)[entry.File] = entry.Path
	}
	return config, validateTargetConfig(config)
}

func SourceEntries(manifest SourceManifest) []SourceEntry {
	entries := []SourceEntry{}
	for _, group := range sortedStringKeys(manifest.Files) {
		for _, file := range sortedStringKeys(manifest.Files[group]) {
			item := manifest.Files[group][file]
			entries = append(entries, SourceEntry{
				Group: group,
				File:  file,
				Key:   ConfigFileKey(group, file),
				Path:  normalizedConfigPath(item.Path),
				Mode:  item.Mode,
			})
		}
	}
	return entries
}

func TargetEntries(config TargetConfig) []TargetEntry {
	entries := []TargetEntry{}
	for _, group := range sortedStringKeys(config.Files) {
		for _, file := range sortedStringKeys(config.Files[group]) {
			entries = append(entries, TargetEntry{
				Group: group,
				File:  file,
				Key:   ConfigFileKey(group, file),
				Path:  normalizedConfigPath(config.Files[group][file]),
			})
		}
	}
	return entries
}

func ConfigFileKey(group string, file string) string {
	return group + "." + file
}

func rejectUndecoded(label string, keys []toml.Key) error {
	if len(keys) == 0 {
		return nil
	}
	formatted := make([]string, 0, len(keys))
	for _, key := range keys {
		formatted = append(formatted, key.String())
	}
	sort.Strings(formatted)
	return fmt.Errorf("%s contains unknown key %q", label, formatted[0])
}

func validateSourceManifest(manifest SourceManifest) error {
	if manifest.Version != 2 {
		return fmt.Errorf("unsupported source manifest version %d", manifest.Version)
	}
	seenPaths := map[string]string{}
	for group, files := range manifest.Files {
		if err := validateConfigID(group, "source group"); err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("source group %q must define files", group)
		}
		for file, item := range files {
			key := ConfigFileKey(group, file)
			if err := validateConfigID(file, "source file"); err != nil {
				return err
			}
			if item.Path == "" {
				return fmt.Errorf("source file %q must define path", key)
			}
			if err := ValidateConfigPath(item.Path, fmt.Sprintf("source file %q", key)); err != nil {
				return err
			}
			normalized := normalizedConfigPath(item.Path)
			if other, ok := seenPaths[normalized]; ok {
				return fmt.Errorf("duplicate source path %q for %s and %s", normalized, other, key)
			}
			seenPaths[normalized] = key
			switch item.Mode {
			case ModeManaged, ModeTemplate:
			case "":
				return fmt.Errorf("source file %q must define mode", key)
			default:
				return fmt.Errorf("source file %q has invalid mode %q", key, item.Mode)
			}
		}
	}
	return nil
}

func validateTargetConfig(config TargetConfig) error {
	if config.Version != 2 {
		return fmt.Errorf("unsupported target config version %d", config.Version)
	}
	if err := ValidateRepository(config.Source.Repository); err != nil {
		return err
	}
	if err := ValidateRef(config.Source.Ref); err != nil {
		return err
	}
	seenTargets := map[string]string{}
	for group, files := range config.Files {
		if err := validateConfigID(group, "target group"); err != nil {
			return err
		}
		if len(files) == 0 {
			return fmt.Errorf("target group %q must define files", group)
		}
		for file, targetPath := range files {
			key := ConfigFileKey(group, file)
			if err := validateConfigID(file, "target file"); err != nil {
				return err
			}
			if err := ValidateConfigPath(targetPath, fmt.Sprintf("target file %q", key)); err != nil {
				return err
			}
			normalized := normalizedConfigPath(targetPath)
			if other, ok := seenTargets[normalized]; ok {
				return fmt.Errorf("duplicate target path %q for %s and %s", normalized, other, key)
			}
			seenTargets[normalized] = key
		}
	}
	return nil
}

func validateConfigID(id string, label string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("%s id is invalid: %q", label, id)
	}
	return nil
}

func ValidateRepository(repository string) error {
	if strings.TrimSpace(repository) != repository || strings.ContainsAny(repository, " \t\n\r") {
		return fmt.Errorf("repository must be owner/repo: %s", repository)
	}
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("repository must be owner/repo: %s", repository)
	}
	return nil
}

func ValidateRef(ref string) error {
	if ref == "" {
		return errors.New("ref must not be empty")
	}
	return nil
}

func ValidateConfigPath(path string, label string) error {
	if path == "" || strings.TrimSpace(path) != path {
		return fmt.Errorf("%s path is invalid: %q", label, path)
	}
	if strings.Contains(path, "\\") || strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || path == "." {
		return fmt.Errorf("%s path is invalid: %q", label, path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == ".." {
			return fmt.Errorf("%s path is invalid: %q", label, path)
		}
	}
	return nil
}

func ensureTargetGroup(files map[string]map[string]string, group string) map[string]string {
	if files[group] == nil {
		files[group] = map[string]string{}
	}
	return files[group]
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
