package driftline

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func ValidateSyncManifestCreation(root string) error {
	_, err := validateSyncManifestCreation(metadataRoot(root))
	return err
}

func LoadSyncManifest(root string) (SyncManifest, error) {
	root = metadataRoot(root)
	if _, err := inspectMetadataDirectory(root, false); err != nil {
		return SyncManifest{}, err
	}
	if _, _, err := inspectSyncManifest(root, false); err != nil {
		return SyncManifest{}, err
	}

	data, err := os.ReadFile(filepath.Join(root, SyncManifestPath))
	if errors.Is(err, os.ErrNotExist) {
		return SyncManifest{}, syncManifestNotFoundError()
	}
	if err != nil {
		return SyncManifest{}, fmt.Errorf("read Sync manifest %s: %w", SyncManifestPath, err)
	}
	return LoadSyncManifestBytes(data)
}

func prepareSyncManifestCreate(root string, manifest SyncManifest) (commit func() error, cleanup func() error, err error) {
	if err := validateSyncManifest(manifest); err != nil {
		return nil, nil, err
	}
	root = metadataRoot(root)
	metadataExists, err := validateSyncManifestCreation(root)
	if err != nil {
		return nil, nil, err
	}
	if !metadataExists {
		if err := os.Mkdir(filepath.Join(root, MetadataDirectoryPath), 0o755); err != nil {
			return nil, nil, fmt.Errorf("create driftline metadata directory %s: %w", MetadataDirectoryPath, err)
		}
	}
	if err := ValidateSyncManifestCreation(root); err != nil {
		return nil, nil, err
	}

	return prepareSyncManifest(root, manifest, 0o644, func() error {
		return ValidateSyncManifestCreation(root)
	})
}

func prepareSyncManifestRewrite(root string, manifest SyncManifest) (commit func() error, cleanup func() error, err error) {
	if err := validateSyncManifest(manifest); err != nil {
		return nil, nil, err
	}
	root = metadataRoot(root)
	mode, err := validateSyncManifestRewrite(root)
	if err != nil {
		return nil, nil, err
	}

	return prepareSyncManifest(root, manifest, mode, func() error {
		_, err := validateSyncManifestRewrite(root)
		return err
	})
}

func validateSyncManifestCreation(root string) (metadataExists bool, err error) {
	metadataExists, err = inspectMetadataDirectory(root, true)
	if err != nil || !metadataExists {
		return metadataExists, err
	}
	_, manifestExists, err := inspectSyncManifest(root, true)
	if err != nil {
		return true, err
	}
	if manifestExists {
		return true, fmt.Errorf("Sync manifest already exists: %s", SyncManifestPath)
	}
	return true, nil
}

func validateSyncManifestRewrite(root string) (os.FileMode, error) {
	if _, err := inspectMetadataDirectory(root, false); err != nil {
		return 0, err
	}
	info, _, err := inspectSyncManifest(root, false)
	if err != nil {
		return 0, err
	}
	return info.Mode().Perm(), nil
}

func inspectMetadataDirectory(root string, allowMissing bool) (bool, error) {
	info, err := os.Lstat(filepath.Join(root, MetadataDirectoryPath))
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return false, nil
		}
		return false, syncManifestNotFoundError()
	}
	if err != nil {
		return false, fmt.Errorf("inspect driftline metadata directory %s: %w", MetadataDirectoryPath, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("driftline metadata path is not a real directory: %s", MetadataDirectoryPath)
	}
	return true, nil
}

func inspectSyncManifest(root string, allowMissing bool) (os.FileInfo, bool, error) {
	info, err := os.Lstat(filepath.Join(root, SyncManifestPath))
	if errors.Is(err, os.ErrNotExist) {
		if allowMissing {
			return nil, false, nil
		}
		return nil, false, syncManifestNotFoundError()
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect Sync manifest path %s: %w", SyncManifestPath, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("Sync manifest path is not a regular file: %s", SyncManifestPath)
	}
	return info, true, nil
}

func prepareSyncManifest(root string, manifest SyncManifest, mode os.FileMode, revalidate func() error) (func() error, func() error, error) {
	temp, err := os.CreateTemp(filepath.Join(root, MetadataDirectoryPath), ".sync-*.toml")
	if err != nil {
		return nil, nil, fmt.Errorf("create Sync manifest temp file: %w", err)
	}
	tempName := temp.Name()
	cleanup := func() error {
		err := os.Remove(tempName)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if _, err := temp.WriteString(FormatSyncManifest(manifest)); err != nil {
		temp.Close()
		cleanup()
		return nil, nil, fmt.Errorf("write Sync manifest temp file: %w", err)
	}
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		cleanup()
		return nil, nil, fmt.Errorf("chmod Sync manifest temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close Sync manifest temp file: %w", err)
	}

	commit := func() error {
		if err := revalidate(); err != nil {
			return err
		}
		if err := os.Rename(tempName, filepath.Join(root, SyncManifestPath)); err != nil {
			return fmt.Errorf("commit Sync manifest: %w", err)
		}
		return nil
	}
	return commit, cleanup, nil
}

func metadataRoot(root string) string {
	if root == "" {
		return "."
	}
	return root
}

func syncManifestNotFoundError() error {
	return fmt.Errorf("Sync manifest not found: %s", SyncManifestPath)
}
