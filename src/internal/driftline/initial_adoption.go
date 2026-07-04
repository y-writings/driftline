package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InitialAdoptionOptions struct {
	Root                        string
	Source                      SourceClient
	Repository                  string
	Commit                      string
	Manifest                    SourceManifest
	TargetConfig                TargetConfig
	AdoptExistingManagedTargets bool
}

func AdoptInitialTargetRepository(opts InitialAdoptionOptions) error {
	return initialAdoption{opts: opts}.adopt()
}

type initialAdoption struct {
	opts                     InitialAdoptionOptions
	prepareTargetConfigWrite func(path string, config TargetConfig) (func() error, func() error, error)
	writeFileBytes           func(target string, data []byte) error
}

type initialAdoptionTemplate struct {
	targetPath  string
	sourceBytes []byte
}

func (a initialAdoption) adopt() error {
	opts := a.opts
	root := opts.Root
	if root == "" {
		root = "."
	}
	if opts.Source == nil {
		return errors.New("source client is required")
	}

	configPath := filepath.Join(root, TargetConfigPath)
	exists, err := initialAdoptionPathExists(configPath)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("target config already exists: %s", TargetConfigPath)
	}

	templates, err := a.collectTemplates(root)
	if err != nil {
		return err
	}

	prepareTargetConfigWrite := a.prepareTargetConfigWrite
	if prepareTargetConfigWrite == nil {
		prepareTargetConfigWrite = PrepareTargetConfigWrite
	}
	commitTargetConfig, cleanupTargetConfig, err := prepareTargetConfigWrite(configPath, opts.TargetConfig)
	if err != nil {
		return err
	}
	defer cleanupTargetConfig()

	writeFileBytes := a.writeFileBytes
	if writeFileBytes == nil {
		writeFileBytes = WriteFileBytes
	}
	for _, template := range templates {
		if err := writeFileBytes(template.targetPath, template.sourceBytes); err != nil {
			return err
		}
	}
	return commitTargetConfig()
}

func (a initialAdoption) collectTemplates(root string) ([]initialAdoptionTemplate, error) {
	type missingTemplate struct {
		sourcePath string
		targetPath string
	}

	missingTemplates := []missingTemplate{}
	templates := []initialAdoptionTemplate{}
	for _, entry := range SourceEntries(a.opts.Manifest) {
		if IsReservedTargetPath(entry.Path) {
			return nil, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := PathWithin(root, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return nil, err
		}
		info, exists, err := initialAdoptionPathInfo(targetPath)
		if err != nil {
			return nil, err
		}

		switch entry.Mode {
		case ModeManaged:
			if err := initialAdoptionRejectSymlinkAncestors(root, targetPath, entry.Path); err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("managed target is not a regular file: %s", entry.Path)
			}
			if !a.opts.AdoptExistingManagedTargets {
				return nil, fmt.Errorf("managed target already exists: %s (rerun with --force to adopt existing regular files)", entry.Path)
			}
		case ModeTemplate:
			if exists {
				continue
			}
			if err := initialAdoptionRejectSymlinkAncestors(root, targetPath, entry.Path); err != nil {
				return nil, err
			}
			missingTemplates = append(missingTemplates, missingTemplate{sourcePath: entry.Path, targetPath: targetPath})
		}
	}
	for _, template := range missingTemplates {
		data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, template.sourcePath)
		if err != nil {
			return nil, fmt.Errorf("source template not found in source repository: %w", err)
		}
		templates = append(templates, initialAdoptionTemplate{targetPath: template.targetPath, sourceBytes: data})
	}
	return templates, nil
}

func initialAdoptionRejectSymlinkAncestors(root string, path string, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return nil
	}

	current := rootAbs
	for _, part := range strings.Split(dir, string(os.PathSeparator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("target path contains symlink: %s", target)
		}
		if !info.IsDir() {
			return fmt.Errorf("target path parent is not a directory: %s", target)
		}
	}
	return nil
}

func initialAdoptionPathInfo(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		return info, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return nil, false, err
}

func initialAdoptionPathExists(path string) (bool, error) {
	_, exists, err := initialAdoptionPathInfo(path)
	return exists, err
}
