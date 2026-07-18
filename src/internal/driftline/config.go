package driftline

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	MetadataDirectoryPath = ".driftline"
	ContractPath          = MetadataDirectoryPath + "/contract.toml"
	SyncManifestPath      = MetadataDirectoryPath + "/sync.toml"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func LoadContractBytes(data []byte) (Contract, error) {
	var contract Contract
	metadata, err := toml.Decode(string(data), &contract)
	if err != nil {
		return contract, fmt.Errorf("parse Contract: %w", err)
	}
	if err := rejectUndecoded("Contract", metadata.Undecoded()); err != nil {
		return contract, err
	}
	return contract, validateContract(contract)
}

func LoadSyncManifestBytes(data []byte) (SyncManifest, error) {
	var manifest SyncManifest
	metadata, err := toml.Decode(string(data), &manifest)
	if err != nil {
		return manifest, fmt.Errorf("parse Sync manifest: %w", err)
	}
	if err := rejectUndecoded("Sync manifest", metadata.Undecoded()); err != nil {
		return manifest, err
	}
	return manifest, validateSyncManifest(manifest)
}

func FormatSyncManifest(manifest SyncManifest) string {
	var b strings.Builder
	b.WriteString("version = 2\n\n")
	b.WriteString("[source]\n")
	b.WriteString("repository = ")
	b.WriteString(strconv.Quote(manifest.Source.Repository))
	b.WriteString("\n")
	b.WriteString("ref = ")
	b.WriteString(strconv.Quote(manifest.Source.Ref))
	b.WriteString("\n")

	for _, group := range sortedStringKeys(manifest.Files) {
		files := manifest.Files[group]
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

func SyncManifestFromContract(repository string, ref string, contract Contract) (SyncManifest, error) {
	if err := ValidateRepository(repository); err != nil {
		return SyncManifest{}, err
	}
	if err := ValidateRef(ref); err != nil {
		return SyncManifest{}, err
	}
	manifest := SyncManifest{
		Version: 2,
		Source: SyncSource{
			Repository: repository,
			Ref:        ref,
		},
		Files: map[string]map[string]string{},
	}
	for _, entry := range ContractEntries(contract) {
		if entry.Mode != ModeManaged {
			continue
		}
		ensureSyncGroup(manifest.Files, entry.Group)[entry.File] = entry.Path
	}
	return manifest, validateSyncManifest(manifest)
}

func ContractEntries(contract Contract) []ContractEntry {
	entries := []ContractEntry{}
	for _, group := range sortedStringKeys(contract.Files) {
		for _, file := range sortedStringKeys(contract.Files[group]) {
			item := contract.Files[group][file]
			entries = append(entries, ContractEntry{
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

func SyncEntries(manifest SyncManifest) []SyncEntry {
	entries := []SyncEntry{}
	for _, group := range sortedStringKeys(manifest.Files) {
		for _, file := range sortedStringKeys(manifest.Files[group]) {
			entries = append(entries, SyncEntry{
				Group: group,
				File:  file,
				Key:   ConfigFileKey(group, file),
				Path:  normalizedConfigPath(manifest.Files[group][file]),
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

func validateContract(contract Contract) error {
	if contract.Version != 2 {
		return fmt.Errorf("unsupported Contract version %d", contract.Version)
	}
	seenPaths := map[string]string{}
	for group, files := range contract.Files {
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
			if err := validateUnreservedMetadataPath(item.Path, fmt.Sprintf("Contract file %q", key)); err != nil {
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

func validateSyncManifest(manifest SyncManifest) error {
	if manifest.Version != 2 {
		return fmt.Errorf("unsupported Sync manifest version %d", manifest.Version)
	}
	if err := ValidateRepository(manifest.Source.Repository); err != nil {
		return err
	}
	if err := ValidateRef(manifest.Source.Ref); err != nil {
		return err
	}
	seenTargets := map[string]string{}
	for group, files := range manifest.Files {
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
			if err := validateUnreservedMetadataPath(targetPath, fmt.Sprintf("Sync manifest file %q", key)); err != nil {
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

func IsReservedMetadataPath(name string) bool {
	name = normalizedConfigPath(name)
	return name == MetadataDirectoryPath || strings.HasPrefix(name, MetadataDirectoryPath+"/")
}

func validateUnreservedMetadataPath(name string, label string) error {
	if IsReservedMetadataPath(name) {
		return fmt.Errorf("reserved driftline metadata path: %s: %s", name, label)
	}
	return nil
}

func ensureSyncGroup(files map[string]map[string]string, group string) map[string]string {
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
