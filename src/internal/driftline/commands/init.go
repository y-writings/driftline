package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runInit(source driftline.SourceClient, opts InitOptions, stdout io.Writer) error {
	if opts.TargetDir == "" {
		opts.TargetDir = "."
	}
	if err := driftline.ValidateRepository(opts.Repository); err != nil {
		return err
	}
	info, err := os.Stat(opts.TargetDir)
	if err != nil {
		return fmt.Errorf("target directory must exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("target directory must be a directory: %s", opts.TargetDir)
	}
	configPath := filepath.Join(opts.TargetDir, driftline.TargetConfigPath)
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("target config already exists: %s", driftline.TargetConfigPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ref := opts.Ref
	commit := ""
	if ref == "" {
		var err error
		ref, commit, err = source.ResolveDefaultRef(opts.Repository)
		if err != nil {
			return err
		}
	} else {
		if err := driftline.ValidateRef(ref); err != nil {
			return err
		}
		var err error
		commit, err = source.ResolveRef(opts.Repository, ref)
		if err != nil {
			return err
		}
	}
	manifestBytes, err := source.ReadFile(opts.Repository, commit, driftline.SourceManifestPath)
	if err != nil {
		return fmt.Errorf(".driftline-source.toml not found in source repository: %w", err)
	}
	manifest, err := driftline.LoadSourceManifestBytes(manifestBytes)
	if err != nil {
		return err
	}
	config, err := driftline.TargetConfigFromSourceManifest(opts.Repository, ref, manifest)
	if err != nil {
		return err
	}
	templates, err := collectInitialTemplates(source, opts, commit, manifest)
	if err != nil {
		return err
	}
	if err := driftline.WriteTargetConfig(configPath, config); err != nil {
		return err
	}
	for _, template := range templates {
		if err := driftline.WriteFileBytes(template.targetPath, template.sourceBytes); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "created .driftline-target.toml from %s@%s\n", opts.Repository, commit)
	return nil
}

type initialTemplate struct {
	targetPath  string
	sourceBytes []byte
}

func collectInitialTemplates(source driftline.SourceClient, opts InitOptions, commit string, manifest driftline.SourceManifest) ([]initialTemplate, error) {
	templates := []initialTemplate{}
	for _, entry := range driftline.SourceEntries(manifest) {
		if entry.Path == driftline.TargetConfigPath {
			return nil, fmt.Errorf("reserved target path %q", entry.Path)
		}
		targetPath, err := driftline.PathWithin(opts.TargetDir, entry.Path, fmt.Sprintf("target %q", entry.Key))
		if err != nil {
			return nil, err
		}
		exists, err := fileExists(targetPath)
		if err != nil {
			return nil, err
		}
		switch entry.Mode {
		case driftline.ModeManaged:
			if exists {
				return nil, fmt.Errorf("managed target already exists: %s", entry.Path)
			}
		case driftline.ModeTemplate:
			if exists {
				continue
			}
			data, err := source.ReadFile(opts.Repository, commit, entry.Path)
			if err != nil {
				return nil, fmt.Errorf("source template not found in source repository: %w", err)
			}
			templates = append(templates, initialTemplate{targetPath: targetPath, sourceBytes: data})
		}
	}
	return templates, nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
