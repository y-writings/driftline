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
	templates := []initialAdoptionTemplate{}
	for _, entry := range SourceEntries(a.opts.Manifest) {
		if IsReservedTargetPath(entry.Path) {
			return nil, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := PathWithin(root, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return nil, err
		}
		exists, err := initialAdoptionPathExists(targetPath)
		if err != nil {
			return nil, err
		}

		switch entry.Mode {
		case ModeManaged:
			if exists {
				return nil, fmt.Errorf("managed target already exists: %s", entry.Path)
			}
		case ModeTemplate:
			if exists {
				continue
			}
			data, err := a.opts.Source.ReadFile(a.opts.Repository, a.opts.Commit, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("source template not found in source repository: %w", err)
			}
			templates = append(templates, initialAdoptionTemplate{targetPath: targetPath, sourceBytes: data})
		}
	}
	return templates, nil
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
