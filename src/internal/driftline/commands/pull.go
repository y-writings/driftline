package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runPull(opts driftline.Options, stdout io.Writer) error { return syncPull(opts, stdout, false) }
func runUpdate(opts driftline.Options, stdout io.Writer) error { return syncPull(opts, stdout, true) }

func syncPull(opts driftline.Options, stdout io.Writer, refresh bool) error {
	cfg, err := driftline.LoadPullConfigPublic(opts.ManifestPath)
	if err != nil { return err }
	lock, err := driftline.LoadLockPublic(opts.LockPath)
	if err != nil { return err }
	byRepo := map[string]driftline.LockedRepo{}
	for _, lr := range lock.Repos { byRepo[lr.Repo] = lr }
	var next []driftline.LockedRepo
	for _, src := range cfg.Pull {
		gitURL := driftline.MapRepoToGitURLPublic(src.Repo)
		tmp, _ := os.MkdirTemp("", "driftline-*")
		defer os.RemoveAll(tmp)
		if err := exec.Command("git", "clone", "--quiet", gitURL, tmp).Run(); err != nil { return err }
		ref := "main"
		if cur, ok := byRepo[src.Repo]; ok && !refresh && cur.From != "" {
			parts := strings.Split(cur.From, "@")
			ref = parts[len(parts)-1]
		}
		if refresh { ref = "main" }
		if err := exec.Command("git", "-C", tmp, "checkout", "--quiet", ref).Run(); err != nil { return err }
		hashBytes, err := exec.Command("git", "-C", tmp, "rev-parse", "HEAD").Output(); if err != nil { return err }
		hash := strings.TrimSpace(string(hashBytes))
		exportCfg, err := driftline.LoadExportConfigPublic(filepath.Join(tmp, ".driftline.export.yaml")); if err != nil { return err }
		rec := driftline.LockedRepo{Repo: src.Repo, From: src.Repo + "@" + hash, Exports: map[string][]string{}}
		for _, name := range src.Exports {
			paths := exportCfg[name]
			rec.Exports[name] = append([]string(nil), paths...)
			for _, rel := range paths {
				srcPath := filepath.Join(tmp, rel)
				dstPath := filepath.Join(opts.TargetDir, rel)
				if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil { return err }
				b, err := os.ReadFile(srcPath); if err != nil { return err }
				if err := os.WriteFile(dstPath, b, 0o644); err != nil { return err }
				fmt.Fprintf(stdout, "copied %s:%s\n", src.Repo, rel)
			}
		}
		next = append(next, rec)
	}
	return driftline.WriteLock(opts.LockPath, driftline.LockFile{Repos: next})
}
