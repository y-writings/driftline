package commands

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runPull(opts driftline.Options, stdout io.Writer) error {
	pullPath := filepath.Join(opts.TargetDir, opts.ManifestPath)
	repos, err := driftline.LoadPull(pullPath)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		return fmt.Errorf("no pull repositories defined")
	}
	repo := repos[0]
	exportPath := filepath.Join(opts.SourceDir, ".driftline.export.yaml")
	units, err := driftline.LoadExportUnits(exportPath)
	if err != nil {
		return err
	}
	lock := driftline.LockFile{Repository: repo.Repo, Ref: opts.Ref, Files: map[string]driftline.LockItem{}}
	for _, unit := range repo.Units {
		files, ok := units[unit]
		if !ok {
			return fmt.Errorf("export unit not found: %s", unit)
		}
		for _, rel := range files {
			src, err := driftline.PathWithin(opts.SourceDir, rel, unit+" source")
			if err != nil { return err }
			dst, err := driftline.PathWithin(opts.TargetDir, rel, unit+" target")
			if err != nil { return err }
			if err := driftline.CopyFile(src, dst); err != nil {
				return err
			}
			hash, _, err := driftline.FileHash(src)
			if err != nil { return err }
			id := unit + ":" + strings.ReplaceAll(rel, "/", "_")
			lock.Files[id] = driftline.LockItem{Target: rel, SourceSHA256: hash}
			fmt.Fprintf(stdout, "add %s %s\n", unit, rel)
		}
	}
	lockPath := filepath.Join(opts.TargetDir, opts.LockPath)
	return driftline.WriteLock(lockPath, lock)
}
