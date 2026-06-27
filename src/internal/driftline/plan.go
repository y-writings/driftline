package driftline

import (
	"fmt"
	"path/filepath"
	"strings"
)

type PlanOptions struct {
	TargetDir string
	Source    SourceClient
	ForceKey  string
}

type Plan struct {
	Repository string
	Ref        string
	Commit     string
	Config     TargetConfig
	Manifest   SourceManifest
	Changes    []Change
	NextConfig TargetConfig
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

	configPath := filepath.Join(opts.TargetDir, TargetConfigPath)
	config, err := LoadTargetConfig(configPath)
	if err != nil {
		return Plan{}, err
	}
	commit, err := opts.Source.ResolveRef(config.Source.Repository, config.Source.Ref)
	if err != nil {
		return Plan{}, err
	}
	manifestBytes, err := opts.Source.ReadFile(config.Source.Repository, commit, SourceManifestPath)
	if err != nil {
		return Plan{}, fmt.Errorf(".driftline-source.toml not found in source repository: %w", err)
	}
	manifest, err := LoadSourceManifestBytes(manifestBytes)
	if err != nil {
		return Plan{}, err
	}

	builder := planBuilder{opts: opts, config: config, manifest: manifest, commit: commit}
	return builder.build()
}

type planBuilder struct {
	opts     PlanOptions
	config   TargetConfig
	manifest SourceManifest
	commit   string
}

type resolvedManagedFile struct {
	SourceEntry
	target     string
	declared   bool
	staleOwner string
}

func (b planBuilder) build() (Plan, error) {
	sourceByKey := map[string]SourceEntry{}
	desiredManagedKeys := map[string]struct{}{}
	managed := []SourceEntry{}
	for _, entry := range SourceEntries(b.manifest) {
		sourceByKey[entry.Key] = entry
		if entry.Mode == ModeManaged {
			managed = append(managed, entry)
			desiredManagedKeys[entry.Key] = struct{}{}
		}
	}

	targetByKey := map[string]TargetEntry{}
	declaredTargets := map[string]string{}
	for _, entry := range TargetEntries(b.config) {
		targetByKey[entry.Key] = entry
		declaredTargets[entry.Path] = entry.Key
	}

	plan := Plan{
		Repository: b.config.Source.Repository,
		Ref:        b.config.Source.Ref,
		Commit:     b.commit,
		Config:     b.config,
		Manifest:   b.manifest,
		NextConfig: TargetConfig{
			Version: b.config.Version,
			Source:  b.config.Source,
			Files:   map[string]map[string]string{},
		},
	}

	usedTargets := map[string]string{}
	forceMatched := b.opts.ForceKey == ""
	for _, entry := range managed {
		if entry.Key == b.opts.ForceKey {
			forceMatched = true
		}
		resolved := resolvedManagedFile{SourceEntry: entry, target: entry.Path}
		if target, ok := targetByKey[entry.Key]; ok {
			resolved.target = target.Path
			resolved.declared = true
		}
		if isReservedTargetPath(resolved.target) {
			return Plan{}, fmt.Errorf("reserved target path %q", resolved.target)
		}
		if other, ok := usedTargets[resolved.target]; ok {
			plan.Changes = append(plan.Changes, conflictChange(resolved, "target already declared by "+other, false))
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
		if err := b.addManagedChange(&plan, resolved); err != nil {
			return Plan{}, err
		}
	}
	if !forceMatched {
		return Plan{}, fmt.Errorf("force key %q does not match a managed source file", b.opts.ForceKey)
	}

	for _, target := range TargetEntries(b.config) {
		if owner, ok := usedTargets[target.Path]; ok {
			if owner != target.Key {
				plan.Changes = append(plan.Changes, targetConfigRemoveChange(target))
			}
			continue
		}
		source, existsInSource := sourceByKey[target.Key]
		if existsInSource && source.Mode == ModeTemplate {
			plan.Changes = append(plan.Changes, Change{
				ID:     target.Key,
				Target: target.Path,
				Status: StatusModeTransition,
				Reason: "source mode changed from managed to template",
			})
			plan.Changes = append(plan.Changes, targetConfigRemoveChange(target))
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
			Reason:        "managed file removed from source config",
			DeletesTarget: true,
		})
		plan.Changes = append(plan.Changes, targetConfigRemoveChange(target))
	}

	if len(plan.Changes) == 0 {
		plan.Changes = append(plan.Changes, Change{Status: StatusSynced})
	}
	return plan, nil
}

func (b planBuilder) addManagedChange(plan *Plan, file resolvedManagedFile) error {
	targetPath, err := PathWithin(b.opts.TargetDir, file.target, fmt.Sprintf("target %q", file.Key))
	if err != nil {
		return err
	}
	currentHash, targetExists, err := FileHash(targetPath)
	if err != nil {
		return fmt.Errorf("hash target %s: %w", file.target, err)
	}
	if !file.declared && file.staleOwner == "" && targetExists && b.opts.ForceKey != file.Key {
		plan.Changes = append(plan.Changes, conflictChange(file, "target already exists", true))
		return nil
	}

	ensureTargetGroup(plan.NextConfig.Files, file.Group)[file.File] = file.target
	if !file.declared {
		plan.Changes = append(plan.Changes, Change{
			ID:     file.Key,
			Target: file.target,
			Status: StatusTargetConfigAdd,
			Reason: "add target config entry",
		})
	}

	sourceBytes, err := b.opts.Source.ReadFile(b.config.Source.Repository, b.commit, file.Path)
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

func conflictChange(file resolvedManagedFile, reason string, forceAllowed bool) Change {
	return Change{
		ID:           file.Key,
		Target:       file.target,
		Status:       StatusConflict,
		Reason:       reason,
		ForceAllowed: forceAllowed,
	}
}

func targetConfigRemoveChange(target TargetEntry) Change {
	return Change{
		ID:     target.Key,
		Target: target.Path,
		Status: StatusTargetConfigRemove,
		Reason: "remove target config entry",
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

func isReservedTargetPath(target string) bool {
	target = normalizedConfigPath(target)
	return target == TargetConfigPath
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
