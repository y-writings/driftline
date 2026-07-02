package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type InitialAdoptionOptions struct {
	Root         string
	Source       SourceClient
	Repository   string
	Commit       string
	Manifest     SourceManifest
	TargetConfig TargetConfig
	ForceKey     string
}

func AdoptInitialTargetRepository(opts InitialAdoptionOptions) error {
	return initialAdoption{opts: opts}.adopt()
}

type initialAdoption struct {
	opts                     InitialAdoptionOptions
	prepareTargetConfigWrite func(path string, config TargetConfig) (func() error, func() error, error)
	writeFileBytes           func(target string, data []byte) error
}

type initialAdoptionWrite struct {
	targetPath  string
	sourceBytes []byte
}

type initialAdoptionWrites struct {
	templates     []initialAdoptionWrite
	forcedManaged []initialAdoptionWrite
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
	if opts.ForceKey != "" {
		if err := validateForceKey(opts.ForceKey); err != nil {
			return err
		}
	}

	configPath := filepath.Join(root, TargetConfigPath)
	exists, err := initialAdoptionPathExists(configPath)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("target config already exists: %s", TargetConfigPath)
	}

	writes, err := a.collectWrites(root)
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
	for _, write := range writes.templates {
		if err := writeFileBytes(write.targetPath, write.sourceBytes); err != nil {
			return err
		}
	}
	if err := commitTargetConfig(); err != nil {
		return err
	}
	for _, write := range writes.forcedManaged {
		if err := writeFileBytes(write.targetPath, write.sourceBytes); err != nil {
			return err
		}
	}
	return nil
}

func (a initialAdoption) collectWrites(root string) (initialAdoptionWrites, error) {
	writes := initialAdoptionWrites{}
	forceMatched := a.opts.ForceKey == ""
	for _, entry := range SourceEntries(a.opts.Manifest) {
		if entry.Mode == ModeManaged && entry.Key == a.opts.ForceKey {
			forceMatched = true
		}
		if IsReservedTargetPath(entry.Path) {
			return initialAdoptionWrites{}, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := PathWithin(root, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return initialAdoptionWrites{}, err
		}
		info, err := os.Lstat(targetPath)
		exists := err == nil
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return initialAdoptionWrites{}, err
			}
		}

		switch entry.Mode {
		case ModeManaged:
			if exists {
				if entry.Key != a.opts.ForceKey || info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
					return initialAdoptionWrites{}, fmt.Errorf("managed target already exists: %s", entry.Path)
				}
				data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, entry.Path)
				if err != nil {
					return initialAdoptionWrites{}, fmt.Errorf("source file not found in source repository: %w", err)
				}
				writes.forcedManaged = append(writes.forcedManaged, initialAdoptionWrite{targetPath: targetPath, sourceBytes: data})
			}
		case ModeTemplate:
			if exists {
				continue
			}
			data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, entry.Path)
			if err != nil {
				return initialAdoptionWrites{}, fmt.Errorf("source template not found in source repository: %w", err)
			}
			writes.templates = append(writes.templates, initialAdoptionWrite{targetPath: targetPath, sourceBytes: data})
		}
	}
	if !forceMatched {
		return initialAdoptionWrites{}, fmt.Errorf("force key %q does not match a managed source file", a.opts.ForceKey)
	}
	return writes, nil
}

func initialAdoptionPathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
