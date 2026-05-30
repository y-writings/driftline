package driftline

import (
	"fmt"
	"path/filepath"
)

type PlanOptions struct {
	TargetDir string
	Source    SourceClient
}

type Plan struct {
	Repository string
	Ref        string
	Commit     string
	Config     TargetConfig
	Manifest   SourceManifest
	Lock       LockFile
	HadLock    bool
	Changes    []Change
	NextLock   LockFile
	GitIgnore  []string
}

func (p Plan) NextLockItem(id string, target string) LockItem {
	for _, item := range p.NextLock.Files {
		if item.ID == id && item.Target == target {
			return item
		}
	}
	return LockItem{}
}

func BuildPlan(opts PlanOptions) (Plan, error) {
	if opts.TargetDir == "" {
		opts.TargetDir = "."
	}
	if opts.Source == nil {
		return Plan{}, fmt.Errorf("source client is required")
	}

	configPath := filepath.Join(opts.TargetDir, TargetConfigPath)
	lockPath := filepath.Join(opts.TargetDir, LockFilePath)
	config, err := LoadTargetConfig(configPath)
	if err != nil {
		return Plan{}, err
	}
	commit, err := opts.Source.ResolveRef(config.Source.Repository, config.Source.Ref)
	if err != nil {
		return Plan{}, err
	}
	manifestBytes, err := opts.Source.ReadFile(config.Source.Repository, commit, TargetConfigPath)
	if err != nil {
		return Plan{}, fmt.Errorf("driftline.yaml not found in source repository: %w", err)
	}
	manifest, err := LoadSourceManifestBytes(manifestBytes)
	if err != nil {
		return Plan{}, err
	}
	lock, hadLock, err := LoadLock(lockPath)
	if err != nil {
		return Plan{}, err
	}

	builder := planBuilder{opts: opts, config: config, manifest: manifest, lock: lock, hadLock: hadLock, commit: commit}
	return builder.build()
}

type planBuilder struct {
	opts     PlanOptions
	config   TargetConfig
	manifest SourceManifest
	lock     LockFile
	hadLock  bool
	commit   string
}

type resolvedFile struct {
	id          string
	source      string
	target      string
	ifNotExists bool
}

func (b planBuilder) build() (Plan, error) {
	manifestByID := map[string]SourceManifestFile{}
	for _, item := range b.manifest.Files {
		manifestByID[item.ID] = item
	}

	activeTargets := map[string]struct{}{}
	lockByIdentity := map[string]LockItem{}
	for _, item := range b.lock.Files {
		lockByIdentity[lockIdentity(item.ID, normalizedTargetPath(item.Target))] = item
	}

	plan := Plan{
		Repository: b.config.Source.Repository,
		Ref:        b.config.Source.Ref,
		Commit:     b.commit,
		Config:     b.config,
		Manifest:   b.manifest,
		Lock:       b.lock,
		HadLock:    b.hadLock,
		GitIgnore:  b.manifest.GitIgnore,
		NextLock: LockFile{
			Version:    1,
			Repository: b.config.Source.Repository,
			Ref:        b.config.Source.Ref,
			Commit:     b.commit,
			Files:      []LockItem{},
		},
	}

	if !b.hadLock {
		plan.Changes = append(plan.Changes, Change{ID: "lock", Status: StatusUpdate, Reason: "lock file is missing"})
	} else if b.lock.Repository != b.config.Source.Repository || b.lock.Ref != b.config.Source.Ref || b.lock.Commit != b.commit {
		plan.Changes = append(plan.Changes, Change{ID: "lock", Status: StatusUpdate, Reason: "source commit changed to " + b.commit})
	}

	resolvedFiles := make([]resolvedFile, 0, len(b.config.Files))
	for _, configured := range b.config.Files {
		manifestItem, ok := manifestByID[configured.ID]
		if !ok {
			return Plan{}, fmt.Errorf("unknown source file id %q", configured.ID)
		}
		resolved := resolveTargetConfigFile(configured, manifestItem)
		if isReservedTargetPath(resolved.target) {
			return Plan{}, fmt.Errorf("reserved target path %q", resolved.target)
		}
		if _, ok := activeTargets[resolved.target]; ok {
			return Plan{}, fmt.Errorf("duplicate target %q", resolved.target)
		}
		activeTargets[resolved.target] = struct{}{}
		resolvedFiles = append(resolvedFiles, resolved)
	}

	for _, resolved := range resolvedFiles {
		sourceBytes, err := b.opts.Source.ReadFile(b.config.Source.Repository, b.commit, resolved.source)
		if err != nil {
			return Plan{}, fmt.Errorf("source file not found in source repository: %w", err)
		}
		sourceHash := HashBytes(sourceBytes)
		targetPath, err := PathWithin(b.opts.TargetDir, resolved.target, fmt.Sprintf("target %q", resolved.id))
		if err != nil {
			return Plan{}, err
		}
		currentHash, targetExists, err := FileHash(targetPath)
		if err != nil {
			return Plan{}, fmt.Errorf("hash target %s: %w", resolved.target, err)
		}

		locked := lockByIdentity[lockIdentity(resolved.id, resolved.target)]
		change := activeChange(resolved, sourceBytes, sourceHash, targetPath, currentHash, targetExists, locked)
		plan.NextLock.Files = append(plan.NextLock.Files, nextActiveLockItem(resolved, sourceHash, currentHash, targetExists, locked))
		plan.Changes = append(plan.Changes, change)
	}

	for _, item := range b.lock.Files {
		if _, ok := activeTargets[normalizedTargetPath(item.Target)]; ok {
			continue
		}
		plan.NextLock.Files = append(plan.NextLock.Files, item)
		change, err := b.staleChange(item)
		if err != nil {
			return Plan{}, err
		}
		plan.Changes = append(plan.Changes, change)
	}

	return plan, nil
}

func resolveTargetConfigFile(configured TargetConfigFile, manifestItem SourceManifestFile) resolvedFile {
	target := configured.Target
	if target == "" {
		target = manifestItem.Target
	}
	ifNotExists := manifestItem.IfNotExists
	if configured.IfNotExists != nil {
		ifNotExists = *configured.IfNotExists
	}
	target = normalizedTargetPath(target)
	return resolvedFile{id: configured.ID, source: manifestItem.Source, target: target, ifNotExists: ifNotExists}
}

func activeChange(file resolvedFile, sourceBytes []byte, sourceHash string, targetPath string, currentHash string, targetExists bool, locked LockItem) Change {
	change := Change{
		ID:           file.id,
		Target:       file.target,
		SourcePath:   file.source,
		TargetPath:   targetPath,
		SourceBytes:  sourceBytes,
		SourceHash:   sourceHash,
		CurrentHash:  currentHash,
		LockedSource: locked.SourceSHA256,
		LockedTarget: locked.TargetSHA256,
		Status:       StatusSynced,
	}
	switch {
	case !targetExists:
		change.Status = StatusAdd
		change.Reason = "target file is missing"
		change.WritesTarget = true
	case file.ifNotExists:
		change.PreservesTarget = true
		if locked.SourceSHA256 != sourceHash {
			change.Status = StatusUpdate
			change.Reason = "target preserved because if_not_exists is enabled"
		}
	case currentHash != sourceHash:
		change.Status = StatusUpdate
		change.Reason = "target differs from source"
		change.WritesTarget = true
	}
	return change
}

func nextActiveLockItem(file resolvedFile, sourceHash string, currentHash string, targetExists bool, locked LockItem) LockItem {
	targetHash := sourceHash
	if file.ifNotExists && targetExists {
		targetHash = currentHash
		if locked.TargetSHA256 != "" {
			targetHash = locked.TargetSHA256
		}
	}
	return LockItem{
		ID:           file.id,
		Target:       file.target,
		SourceSHA256: sourceHash,
		TargetSHA256: targetHash,
	}
}

func (b planBuilder) staleChange(item LockItem) (Change, error) {
	targetPath, err := PathWithin(b.opts.TargetDir, item.Target, fmt.Sprintf("locked target %q", item.ID))
	if err != nil {
		return Change{}, err
	}
	currentHash, targetExists, err := FileHash(targetPath)
	if err != nil {
		return Change{}, fmt.Errorf("hash stale target %s: %w", item.Target, err)
	}
	change := Change{
		ID:           item.ID,
		Target:       item.Target,
		TargetPath:   targetPath,
		CurrentHash:  currentHash,
		LockedSource: item.SourceSHA256,
		LockedTarget: item.TargetSHA256,
		Status:       StatusPrune,
		Reason:       "target is no longer adopted",
	}
	if targetExists && currentHash != item.TargetSHA256 {
		change.Status = StatusConflict
		change.Reason = "target has local changes"
	}
	return change, nil
}

func lockIdentity(id string, target string) string {
	return id + "\x00" + target
}

func isReservedTargetPath(target string) bool {
	target = normalizedTargetPath(target)
	return target == TargetConfigPath || target == LockFilePath
}

func normalizedTargetPath(target string) string {
	return filepath.ToSlash(filepath.Clean(target))
}

func HasDrift(changes []Change) bool {
	for _, change := range changes {
		if change.Status != StatusSynced {
			return true
		}
	}
	return false
}
