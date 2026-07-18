package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type PlanOptions struct {
	TargetDir string
	Source    SourceClient
	ForceKey  string
}

type Plan struct {
	Repository       string
	Ref              string
	Commit           string
	SyncManifest     SyncManifest
	Contract         Contract
	Changes          []Change
	NextSyncManifest SyncManifest
}

func (p Plan) HasConflicts() bool {
	for _, change := range p.Changes {
		if change.Status == StatusConflict {
			return true
		}
	}
	return false
}

func BuildPlan(opts PlanOptions) (Plan, error) {
	if opts.TargetDir == "" {
		opts.TargetDir = "."
	}
	if opts.Source == nil {
		return Plan{}, fmt.Errorf("source client is required")
	}
	if opts.ForceKey != "" {
		if err := validateForceKey(opts.ForceKey); err != nil {
			return Plan{}, err
		}
	}

	syncManifestPath := filepath.Join(opts.TargetDir, TargetConfigPath)
	syncManifest, err := LoadTargetConfig(syncManifestPath)
	if err != nil {
		return Plan{}, err
	}
	commit, err := opts.Source.ResolveRef(syncManifest.Source.Repository, syncManifest.Source.Ref)
	if err != nil {
		return Plan{}, err
	}
	contractBytes, err := opts.Source.ReadFile(syncManifest.Source.Repository, commit, SourceManifestPath)
	if err != nil {
		return Plan{}, fmt.Errorf(".driftline-source.toml not found in source repository: %w", err)
	}
	contract, err := LoadContractBytes(contractBytes)
	if err != nil {
		return Plan{}, err
	}

	builder := planBuilder{opts: opts, syncManifest: syncManifest, contract: contract, commit: commit}
	return builder.build()
}

type planBuilder struct {
	opts         PlanOptions
	syncManifest SyncManifest
	contract     Contract
	commit       string
}

type resolvedManagedFile struct {
	ContractEntry
	target     string
	declared   bool
	staleOwner string
}

func (b planBuilder) build() (Plan, error) {
	contractByKey := map[string]ContractEntry{}
	desiredManagedKeys := map[string]struct{}{}
	managed := []ContractEntry{}
	for _, entry := range ContractEntries(b.contract) {
		contractByKey[entry.Key] = entry
		if entry.Mode == ModeManaged {
			managed = append(managed, entry)
			desiredManagedKeys[entry.Key] = struct{}{}
		}
	}

	syncByKey := map[string]SyncEntry{}
	declaredTargets := map[string]string{}
	for _, entry := range SyncEntries(b.syncManifest) {
		syncByKey[entry.Key] = entry
		declaredTargets[entry.Path] = entry.Key
	}
	staleDeleteTargets := staleDeleteTargetPaths(b.syncManifest, contractByKey)

	plan := Plan{
		Repository:   b.syncManifest.Source.Repository,
		Ref:          b.syncManifest.Source.Ref,
		Commit:       b.commit,
		SyncManifest: b.syncManifest,
		Contract:     b.contract,
		NextSyncManifest: SyncManifest{
			Version: b.syncManifest.Version,
			Source:  b.syncManifest.Source,
			Files:   map[string]map[string]string{},
		},
	}

	usedTargets := map[string]string{}
	forceMatched := b.opts.ForceKey == ""
	for _, entry := range managed {
		if entry.Key == b.opts.ForceKey {
			forceMatched = true
		}
		resolved := resolvedManagedFile{ContractEntry: entry, target: entry.Path}
		if target, ok := syncByKey[entry.Key]; ok {
			resolved.target = target.Path
			resolved.declared = true
		}
		if IsReservedTargetPath(resolved.target) {
			return Plan{}, fmt.Errorf("reserved target path %q", resolved.target)
		}
		if other, ok := usedTargets[resolved.target]; ok {
			plan.Changes = append(plan.Changes, conflictChange(resolved, "target already declared by "+other, false))
			continue
		}
		if other, ok := overlappingManagedTarget(resolved.target, usedTargets); ok {
			plan.Changes = append(plan.Changes, conflictChange(resolved, "target overlaps with "+other, false))
			continue
		}
		if other, ok := declaredTargets[resolved.target]; ok && other != resolved.Key {
			if _, desired := desiredManagedKeys[other]; desired {
				plan.Changes = append(plan.Changes, conflictChange(resolved, "target already declared by "+other, false))
				continue
			}
			resolved.staleOwner = other
		}
		usedTargets[resolved.target] = resolved.Key
		if err := b.addManagedChange(&plan, resolved, staleDeleteTargets); err != nil {
			return Plan{}, err
		}
	}
	if !forceMatched {
		return Plan{}, fmt.Errorf("force key %q does not match a managed source file", b.opts.ForceKey)
	}

	for _, target := range SyncEntries(b.syncManifest) {
		if owner, ok := usedTargets[target.Path]; ok {
			if owner != target.Key {
				plan.Changes = append(plan.Changes, syncManifestRemoveChange(target))
			}
			continue
		}
		source, existsInSource := contractByKey[target.Key]
		if existsInSource && source.Mode == ModeTemplate {
			plan.Changes = append(plan.Changes, Change{
				ID:     target.Key,
				Target: target.Path,
				Status: StatusModeTransition,
				Reason: "source mode changed from managed to template",
			})
			plan.Changes = append(plan.Changes, syncManifestRemoveChange(target))
			continue
		}
		fullPath, err := PathWithin(b.opts.TargetDir, target.Path, fmt.Sprintf("target %q", target.Key))
		if err != nil {
			return Plan{}, err
		}
		plan.Changes = append(plan.Changes, Change{
			ID:            target.Key,
			Target:        target.Path,
			TargetPath:    fullPath,
			Status:        StatusRemove,
			Reason:        "managed file removed from Contract",
			DeletesTarget: true,
		})
		plan.Changes = append(plan.Changes, syncManifestRemoveChange(target))
	}

	if len(plan.Changes) == 0 {
		plan.Changes = append(plan.Changes, Change{Status: StatusSynced})
	}
	return plan, nil
}

func (b planBuilder) addManagedChange(plan *Plan, file resolvedManagedFile, staleDeleteTargets map[string]struct{}) error {
	targetPath, err := PathWithin(b.opts.TargetDir, file.target, fmt.Sprintf("target %q", file.Key))
	if err != nil {
		return err
	}
	currentHash := ""
	targetExists := false
	blockedByStaleFile, err := b.targetBlockedByStaleFileAncestor(file.target, staleDeleteTargets)
	if err != nil {
		return err
	}
	if !blockedByStaleFile {
		info, err := os.Lstat(targetPath)
		if err != nil {
			if errors.Is(err, syscall.ENOTDIR) {
				plan.Changes = append(plan.Changes, conflictChange(file, "target already exists", false))
				return nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("stat target %s: %w", file.target, err)
			}
		} else if info.Mode()&os.ModeSymlink != 0 {
			plan.Changes = append(plan.Changes, conflictChange(file, "target already exists", false))
			return nil
		} else if info.IsDir() {
			if !file.declared {
				plan.Changes = append(plan.Changes, conflictChange(file, "target already exists", false))
				return nil
			}
			return fmt.Errorf("target %s is a directory", file.target)
		} else {
			targetExists = true
		}
	}
	if targetExists {
		currentHash, _, err = FileHash(targetPath)
		if err != nil {
			return fmt.Errorf("hash target %s: %w", file.target, err)
		}
	}
	if !file.declared && targetExists && b.opts.ForceKey != file.Key {
		plan.Changes = append(plan.Changes, conflictChange(file, "target already exists", true))
		return nil
	}

	ensureSyncGroup(plan.NextSyncManifest.Files, file.Group)[file.File] = file.target
	if !file.declared {
		plan.Changes = append(plan.Changes, Change{
			ID:     file.Key,
			Target: file.target,
			Status: StatusSyncManifestAdd,
			Reason: "add Sync manifest entry",
		})
	}

	sourceBytes, err := b.opts.Source.ReadFile(b.syncManifest.Source.Repository, b.commit, file.Path)
	if err != nil {
		return fmt.Errorf("source file not found in source repository: %w", err)
	}
	sourceHash := HashBytes(sourceBytes)
	change := Change{
		ID:          file.Key,
		Target:      file.target,
		TargetPath:  targetPath,
		SourcePath:  file.Path,
		SourceBytes: sourceBytes,
		Status:      StatusSynced,
	}
	switch {
	case !targetExists:
		change.Status = StatusAdd
		change.Reason = "target file is missing"
		change.WritesTarget = true
	case currentHash != sourceHash:
		change.Status = StatusUpdate
		change.Reason = "target differs from source"
		change.WritesTarget = true
	}
	plan.Changes = append(plan.Changes, change)
	return nil
}

func staleDeleteTargetPaths(syncManifest SyncManifest, contractByKey map[string]ContractEntry) map[string]struct{} {
	targets := map[string]struct{}{}
	for _, target := range SyncEntries(syncManifest) {
		if _, ok := contractByKey[target.Key]; !ok {
			targets[target.Path] = struct{}{}
		}
	}
	return targets
}

func (b planBuilder) targetBlockedByStaleFileAncestor(target string, staleDeleteTargets map[string]struct{}) (bool, error) {
	for staleTarget := range staleDeleteTargets {
		if !isPathAncestor(staleTarget, target) {
			continue
		}
		fullPath, err := PathWithin(b.opts.TargetDir, staleTarget, fmt.Sprintf("stale target %q", staleTarget))
		if err != nil {
			return false, err
		}
		info, err := os.Stat(fullPath)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("stat stale target %s: %w", staleTarget, err)
		}
		if !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

func isPathAncestor(parent string, child string) bool {
	parent = normalizedConfigPath(parent)
	child = normalizedConfigPath(child)
	return parent != child && strings.HasPrefix(child, parent+"/")
}

func overlappingManagedTarget(target string, usedTargets map[string]string) (string, bool) {
	for _, usedTarget := range sortedStringKeys(usedTargets) {
		if isPathAncestor(usedTarget, target) || isPathAncestor(target, usedTarget) {
			return usedTargets[usedTarget], true
		}
	}
	return "", false
}

func conflictChange(file resolvedManagedFile, reason string, forceAllowed bool) Change {
	return Change{
		ID:           file.Key,
		Target:       file.target,
		Status:       StatusConflict,
		Reason:       reason,
		ForceAllowed: forceAllowed,
	}
}

func syncManifestRemoveChange(target SyncEntry) Change {
	return Change{
		ID:     target.Key,
		Target: target.Path,
		Status: StatusSyncManifestRemove,
		Reason: "remove Sync manifest entry",
	}
}

func validateForceKey(key string) error {
	group, file, ok := strings.Cut(key, ".")
	if !ok || strings.Contains(file, ".") {
		return fmt.Errorf("force key must be <group.file>: %q", key)
	}
	if err := validateConfigID(group, "force group"); err != nil {
		return err
	}
	if err := validateConfigID(file, "force file"); err != nil {
		return err
	}
	return nil
}

func IsReservedTargetPath(target string) bool {
	target = normalizedConfigPath(target)
	return IsReservedMetadataPath(target) || target == TargetConfigPath || target == removedLockPath
}

func normalizedConfigPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func HasDrift(changes []Change) bool {
	for _, change := range changes {
		if change.Status != StatusSynced {
			return true
		}
	}
	return false
}
