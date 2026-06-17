package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SourceManifestPath = ".driftline-source.yaml"
	TargetConfigPath   = ".driftline-target.yaml"
	LockFilePath       = "driftline-lock.yaml"
)

func (file *SourceManifestFile) UnmarshalYAML(node *yaml.Node) error {
	type sourceManifestFileFields struct {
		ID    string    `yaml:"id"`
		Name  string    `yaml:"name"`
		Paths yaml.Node `yaml:"paths"`
	}

	var fields sourceManifestFileFields
	if err := node.Decode(&fields); err != nil {
		return err
	}
	file.ID = fields.ID
	file.Name = fields.Name
	file.Paths = nil
	if fields.Paths.Kind == 0 {
		return nil
	}
	if fields.Paths.Kind != yaml.MappingNode {
		return fmt.Errorf("source manifest file %q paths must be a mapping", fields.ID)
	}
	file.Paths = make([]SourceManifestPathEntry, 0, len(fields.Paths.Content)/2)
	for i := 0; i < len(fields.Paths.Content)-1; i += 2 {
		idNode := fields.Paths.Content[i]
		pathNode := fields.Paths.Content[i+1]
		if idNode.Kind != yaml.ScalarNode {
			return fmt.Errorf("source manifest file %q path id must be a scalar", fields.ID)
		}
		path, err := decodeSourceManifestPathEntry(fields.ID, idNode.Value, pathNode)
		if err != nil {
			return err
		}
		file.Paths = append(file.Paths, path)
	}
	return nil
}

func decodeSourceManifestPathEntry(fileID string, pathID string, node *yaml.Node) (SourceManifestPathEntry, error) {
	if node.Kind != yaml.MappingNode {
		return SourceManifestPathEntry{}, fmt.Errorf("source manifest file %q path %q must be a mapping", fileID, pathID)
	}
	path := SourceManifestPathEntry{ID: pathID}
	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		switch keyNode.Value {
		case "name":
			if valueNode.Kind != yaml.ScalarNode {
				return SourceManifestPathEntry{}, fmt.Errorf("source manifest file %q path %q name must be a scalar", fileID, pathID)
			}
			path.Name = valueNode.Value
		case "path":
			if valueNode.Kind != yaml.ScalarNode {
				return SourceManifestPathEntry{}, fmt.Errorf("source manifest file %q path %q path must be a scalar", fileID, pathID)
			}
			path.Path = valueNode.Value
		default:
			return SourceManifestPathEntry{}, fmt.Errorf("unknown key %q", keyNode.Value)
		}
	}
	return path, nil
}

func LoadSourceManifestBytes(data []byte) (SourceManifest, error) {
	var manifest SourceManifest
	doc, err := parseStrictYAML(data, allowedSourceManifestKeys())
	if err != nil {
		return manifest, fmt.Errorf("parse source manifest: %w", err)
	}
	if err := doc.Decode(&manifest); err != nil {
		return manifest, fmt.Errorf("parse source manifest: %w", err)
	}
	return manifest, validateSourceManifest(manifest, doc)
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
	doc, err := parseStrictYAML(data, allowedTargetConfigKeys())
	if err != nil {
		return config, fmt.Errorf("parse target config: %w", err)
	}
	if err := doc.Decode(&config); err != nil {
		return config, fmt.Errorf("parse target config: %w", err)
	}
	return config, validateTargetConfig(config, doc)
}

func WriteTargetConfig(path string, config TargetConfig) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode target config: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
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
		Files: make([]TargetConfigFile, 0, len(manifest.Files)),
	}
	seenDefaultTargets := map[string]struct{}{}
	for _, item := range manifest.Files {
		for _, sourcePath := range item.Paths {
			defaultTarget := normalizedConfigPath(sourcePath.Path)
			if _, ok := seenDefaultTargets[defaultTarget]; ok {
				return TargetConfig{}, fmt.Errorf("duplicate target %q", defaultTarget)
			}
			seenDefaultTargets[defaultTarget] = struct{}{}
		}
		config.Files = append(config.Files, TargetConfigFile{ID: item.ID})
	}
	return config, nil
}

func LoadLock(path string) (LockFile, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return LockFile{Version: 1, Files: []LockItem{}}, false, nil
	}
	if err != nil {
		return LockFile{}, false, fmt.Errorf("read lock file: %w", err)
	}
	lock, err := LoadLockBytes(data)
	return lock, true, err
}

func LoadLockBytes(data []byte) (LockFile, error) {
	var lock LockFile
	doc, err := parseStrictYAML(data, allowedLockKeys())
	if err != nil {
		return lock, fmt.Errorf("parse lock file: %w", err)
	}
	if err := doc.Decode(&lock); err != nil {
		return lock, fmt.Errorf("parse lock file: %w", err)
	}
	return lock, validateLock(lock, doc)
}

func WriteLock(path string, lock LockFile) error {
	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode lock file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create lock file directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func allowedSourceManifestKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":      set("version", "gitignore", "files"),
		"files": set("id", "name", "paths"),
		"paths": nil,
	}
}

func allowedTargetConfigKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":               set("version", "source", "files"),
		"source":         set("repository", "ref"),
		"files":          set("id", "path_overrides", "if_not_exists"),
		"path_overrides": nil,
	}
}

func allowedLockKeys() map[string]map[string]struct{} {
	return map[string]map[string]struct{}{
		"":      set("version", "repository", "ref", "commit", "files"),
		"files": set("id", "target_path"),
	}
}

func set(keys ...string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, key := range keys {
		out[key] = struct{}{}
	}
	return out
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
	if strings.Contains(path, "\\") || strings.HasPrefix(path, "/") || path == "." {
		return fmt.Errorf("%s path is invalid: %q", label, path)
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || part == ".." {
			return fmt.Errorf("%s path is invalid: %q", label, path)
		}
	}
	return nil
}

func parseStrictYAML(data []byte, allowed map[string]map[string]struct{}) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	root := documentRoot(&doc)
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, errors.New("document must be a mapping")
	}
	if err := validateNodeKeys(root, "", allowed); err != nil {
		return nil, err
	}
	return root, nil
}

func validateNodeKeys(node *yaml.Node, section string, allowed map[string]map[string]struct{}) error {
	switch node.Kind {
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		allowedKeys, checkUnknown := allowed[section]
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := keyNode.Value
			if _, ok := seen[key]; ok {
				return fmt.Errorf("duplicate key %q", key)
			}
			seen[key] = struct{}{}
			if checkUnknown && allowedKeys != nil {
				if _, ok := allowedKeys[key]; !ok {
					return fmt.Errorf("unknown key %q", key)
				}
			}
			nextSection := section
			if _, ok := allowed[key]; ok {
				nextSection = key
			}
			if err := validateNodeKeys(valueNode, nextSection, allowed); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if err := validateNodeKeys(item, section, allowed); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSourceManifest(manifest SourceManifest, root *yaml.Node) error {
	if manifest.Version != 2 {
		return fmt.Errorf("unsupported source manifest version %d", manifest.Version)
	}
	if err := requireSequence(root, "files", "source manifest"); err != nil {
		return err
	}
	seenIDs := map[string]struct{}{}
	for _, item := range manifest.Files {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("source manifest contains file without id")
		}
		if _, ok := seenIDs[item.ID]; ok {
			return fmt.Errorf("duplicate source manifest file id %q", item.ID)
		}
		seenIDs[item.ID] = struct{}{}
		if len(item.Paths) == 0 {
			return fmt.Errorf("source manifest file %q must define paths", item.ID)
		}
		seenPaths := map[string]struct{}{}
		for _, sourcePath := range item.Paths {
			if strings.TrimSpace(sourcePath.ID) == "" {
				return fmt.Errorf("source manifest file %q contains path without id", item.ID)
			}
			if sourcePath.Path == "" {
				return fmt.Errorf("source manifest file %q path %q must define path", item.ID, sourcePath.ID)
			}
			if err := ValidateConfigPath(sourcePath.Path, fmt.Sprintf("source %q path %q", item.ID, sourcePath.ID)); err != nil {
				return err
			}
			normalized := normalizedConfigPath(sourcePath.Path)
			if _, ok := seenPaths[normalized]; ok {
				return fmt.Errorf("duplicate source path %q in source manifest file id %q", normalized, item.ID)
			}
			seenPaths[normalized] = struct{}{}
		}
	}
	return nil
}

func validateTargetConfig(config TargetConfig, root *yaml.Node) error {
	if config.Version != 2 {
		return fmt.Errorf("unsupported target config version %d", config.Version)
	}
	if err := requireMapping(root, "source", "target config"); err != nil {
		return err
	}
	if err := requireSequence(root, "files", "target config"); err != nil {
		return err
	}
	if err := ValidateRepository(config.Source.Repository); err != nil {
		return err
	}
	if err := ValidateRef(config.Source.Ref); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	fileNodes := configSequenceItems(root, "files")
	for i, item := range config.Files {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("target config contains file without id")
		}
		if _, ok := seen[item.ID]; ok {
			return fmt.Errorf("duplicate target config file id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if i < len(fileNodes) && mappingHasKey(fileNodes[i], "path_overrides") {
			overridesNode := configMappingValue(fileNodes[i], "path_overrides")
			if overridesNode == nil || overridesNode.Kind != yaml.MappingNode {
				return fmt.Errorf("target config file %q path_overrides must be a mapping", item.ID)
			}
			if len(item.PathOverrides) == 0 {
				return fmt.Errorf("target config file %q path_overrides must not be empty", item.ID)
			}
		}
		for pathID, targetPath := range item.PathOverrides {
			if strings.TrimSpace(pathID) == "" {
				return fmt.Errorf("target config file %q contains path override without id", item.ID)
			}
			if err := ValidateConfigPath(targetPath, fmt.Sprintf("target %q override %q", item.ID, pathID)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateLock(lock LockFile, root *yaml.Node) error {
	if lock.Version != 1 {
		return fmt.Errorf("unsupported lock file version %d", lock.Version)
	}
	if err := requireSequence(root, "files", "lock file"); err != nil {
		return err
	}
	if err := ValidateRepository(lock.Repository); err != nil {
		return err
	}
	if err := ValidateRef(lock.Ref); err != nil {
		return err
	}
	if lock.Commit == "" {
		return errors.New("lock file commit must not be empty")
	}
	seenIdentity := map[string]struct{}{}
	seenTarget := map[string]struct{}{}
	for _, item := range lock.Files {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("lock file contains item without id")
		}
		if err := ValidateConfigPath(item.TargetPath, fmt.Sprintf("target %q", item.ID)); err != nil {
			return err
		}
		identity := item.ID + "\x00" + item.TargetPath
		if _, ok := seenIdentity[identity]; ok {
			return fmt.Errorf("duplicate lock item %q target %q", item.ID, item.TargetPath)
		}
		seenIdentity[identity] = struct{}{}
		if _, ok := seenTarget[item.TargetPath]; ok {
			return fmt.Errorf("duplicate target %q", item.TargetPath)
		}
		seenTarget[item.TargetPath] = struct{}{}
	}
	return nil
}

func requireMapping(root *yaml.Node, key string, label string) error {
	node := configMappingValue(root, key)
	if node == nil {
		return fmt.Errorf("%s must define %s", label, key)
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("%s %s must be a mapping", label, key)
	}
	return nil
}

func requireSequence(root *yaml.Node, key string, label string) error {
	node := configMappingValue(root, key)
	if node == nil {
		return fmt.Errorf("%s must define %s", label, key)
	}
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("%s %s must be a sequence", label, key)
	}
	return nil
}

func configSequenceItems(root *yaml.Node, key string) []*yaml.Node {
	node := configMappingValue(root, key)
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	return node.Content
}

func mappingHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func configMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind == yaml.DocumentNode {
		node = documentRoot(node)
	}
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}
