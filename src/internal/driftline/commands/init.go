package commands

import (
	"fmt"
	"io"
	"os"

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
	if err := driftline.AdoptInitialTargetRepository(driftline.InitialAdoptionOptions{
		Root:         opts.TargetDir,
		Source:       source,
		Repository:   opts.Repository,
		Commit:       commit,
		Manifest:     manifest,
		TargetConfig: config,
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created .driftline-target.toml from %s@%s\n", opts.Repository, commit)
	return nil
}
