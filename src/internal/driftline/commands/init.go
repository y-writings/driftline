package commands

import (
	"errors"
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
	if err := driftline.ValidateSyncManifestCreation(opts.TargetDir); err != nil {
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
	contractBytes, err := source.ReadFile(opts.Repository, commit, driftline.ContractPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Contract not found: %s: %w", driftline.ContractPath, err)
		}
		return fmt.Errorf("read Contract %s: %w", driftline.ContractPath, err)
	}
	contract, err := driftline.LoadContractBytes(contractBytes)
	if err != nil {
		return err
	}
	syncManifest, err := driftline.SyncManifestFromContract(opts.Repository, ref, contract)
	if err != nil {
		return err
	}
	if err := driftline.AdoptInitialTargetRepository(driftline.InitialAdoptionOptions{
		Root:                        opts.TargetDir,
		Source:                      source,
		Repository:                  opts.Repository,
		Commit:                      commit,
		Contract:                    contract,
		SyncManifest:                syncManifest,
		AdoptExistingManagedTargets: opts.Force,
	}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created Sync manifest %s from %s@%s\n", driftline.SyncManifestPath, opts.Repository, commit)
	return nil
}
