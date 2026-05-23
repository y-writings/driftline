package driftline

import (
	"fmt"
)

type Options struct {
	ManifestPath string
	LockPath     string
	SourceDir    string
	TargetDir    string
	Repository   string
	Ref          string
}

func DefaultOptions() Options {
	return Options{
		ManifestPath: "driftline.yaml",
		LockPath:     ".driftline.lock",
		SourceDir:    ".",
		TargetDir:    ".",
	}
}

func BuildPlan(opts Options) (Manifest, LockFile, []Change, error) {
	manifestPath, err := pathWithin(opts.SourceDir, opts.ManifestPath, "manifest")
	if err != nil {
		return Manifest{}, LockFile{}, nil, err
	}
	lockPath, err := pathWithin(opts.TargetDir, opts.LockPath, "lock")
	if err != nil {
		return Manifest{}, LockFile{}, nil, err
	}
	manifest, err := loadManifest(manifestPath)
	if err != nil {
		return manifest, LockFile{}, nil, err
	}
	lock, err := loadLock(lockPath)
	if err != nil {
		return manifest, lock, nil, err
	}

	manifestIDs := map[string]ManifestFile{}
	var changes []Change
	for _, item := range manifest.File {
		manifestIDs[item.ID] = item
		sourcePath, err := pathWithin(opts.SourceDir, item.Source, fmt.Sprintf("source %q", item.ID))
		if err != nil {
			return manifest, lock, nil, err
		}
		targetPath, err := pathWithin(opts.TargetDir, item.Target, fmt.Sprintf("target %q", item.ID))
		if err != nil {
			return manifest, lock, nil, err
		}
		sourceHash, sourceExists, err := fileHash(sourcePath)
		if err != nil {
			return manifest, lock, nil, fmt.Errorf("hash source %s: %w", item.Source, err)
		}
		if !sourceExists {
			return manifest, lock, nil, fmt.Errorf("source file does not exist: %s", item.Source)
		}
		currentHash, targetExists, err := fileHash(targetPath)
		if err != nil {
			return manifest, lock, nil, fmt.Errorf("hash target %s: %w", item.Target, err)
		}

		change := Change{
			ID:          item.ID,
			SourcePath:  sourcePath,
			TargetPath:  targetPath,
			SourceHash:  sourceHash,
			CurrentHash: currentHash,
			Status:      StatusSynced,
		}
		if locked, ok := lock.Files[item.ID]; ok {
			change.LockedHash = locked.SourceSHA256
		}
		switch {
		case !targetExists:
			change.Status = StatusAdd
			change.Reason = "target file is missing"
		case item.IfNotExists:
			change.Reason = "if_not_exists is enabled and target exists"
		case currentHash != sourceHash:
			change.Status = StatusUpdate
			change.Reason = "target differs from source"
		}
		changes = append(changes, change)
	}

	for id, item := range lock.Files {
		if _, ok := manifestIDs[id]; ok {
			continue
		}
		targetPath, err := pathWithin(opts.TargetDir, item.Target, fmt.Sprintf("locked target %q", id))
		if err != nil {
			return manifest, lock, nil, err
		}
		currentHash, targetExists, err := fileHash(targetPath)
		if err != nil {
			return manifest, lock, nil, fmt.Errorf("hash stale target %s: %w", item.Target, err)
		}
		change := Change{
			ID:          id,
			TargetPath:  targetPath,
			CurrentHash: currentHash,
			LockedHash:  item.SourceSHA256,
			Status:      StatusPrune,
			Reason:      "source was removed from manifest",
		}
		if targetExists && currentHash != item.SourceSHA256 {
			change.Status = StatusConflict
			change.Reason = "source was removed, but target has local changes"
		}
		changes = append(changes, change)
	}

	return manifest, lock, changes, nil
}

func HasDrift(changes []Change) bool {
	for _, change := range changes {
		if change.Status != StatusSynced {
			return true
		}
	}
	return false
}
