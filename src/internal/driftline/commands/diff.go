package commands

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func runDiff(source driftline.SourceClient, opts TargetOptions, stdout io.Writer) error {
	plan, err := driftline.BuildPlan(driftline.PlanOptions{TargetDir: opts.TargetDir, Source: source})
	if err != nil {
		return err
	}
	for _, change := range sortedChanges(plan.Changes) {
		switch {
		case (change.Status == driftline.StatusAdd || change.Status == driftline.StatusUpdate) && change.WritesTarget:
			if err := printBytesDiff(stdout, change.SourceBytes, change.TargetPath, change.Status == driftline.StatusAdd); err != nil {
				return err
			}
		case change.Status != driftline.StatusSynced:
			printChange(stdout, change)
		}
	}
	if plan.GitIgnore != nil {
		if err := printGitIgnoreDiff(
			stdout,
			plan.GitIgnore.OriginalBytes,
			plan.GitIgnore.DesiredBytes,
			plan.GitIgnore.TargetMissing,
		); err != nil {
			return err
		}
	}
	if plan.HasDrift() {
		return errDrift
	}
	return nil
}

func printBytesDiff(w io.Writer, sourceBytes []byte, targetPath string, targetMissing bool) error {
	temp, err := os.CreateTemp("", "driftline-source-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(sourceBytes); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return printGitDiff(w, temp.Name(), targetPath, targetMissing)
}

func printGitIgnoreDiff(w io.Writer, originalBytes, desiredBytes []byte, targetMissing bool) error {
	tempDir, err := os.MkdirTemp("", "driftline-diff-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	originalPath := filepath.Join(tempDir, "original")
	desiredPath := filepath.Join(tempDir, "desired")
	if err := os.WriteFile(originalPath, originalBytes, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(desiredPath, desiredBytes, 0o600); err != nil {
		return err
	}

	left := "original"
	if targetMissing {
		left = os.DevNull
	}
	cmd := exec.Command("git", "diff", "--no-index", "--no-color", "--no-ext-diff", "--no-textconv", "--", left, "desired")
	cmd.Dir = tempDir
	data, diffErr := cmd.CombinedOutput()
	data = relabelGitIgnoreDiff(data, targetMissing)
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if diffErr == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(diffErr, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("run git diff: %w", diffErr)
}

func relabelGitIgnoreDiff(data []byte, targetMissing bool) []byte {
	var output bytes.Buffer
	inHunk := false
	for _, line := range bytes.SplitAfter(data, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("@@ ")) {
			inHunk = true
		}
		if inHunk {
			output.Write(line)
			continue
		}
		switch {
		case bytes.HasPrefix(line, []byte("diff --git ")):
			fmt.Fprintf(&output, "diff --git a/%s b/%s\n", driftline.GitIgnorePath, driftline.GitIgnorePath)
		case bytes.HasPrefix(line, []byte("--- ")):
			if targetMissing {
				output.WriteString("--- /dev/null\n")
			} else {
				fmt.Fprintf(&output, "--- a/%s\n", driftline.GitIgnorePath)
			}
		case bytes.HasPrefix(line, []byte("+++ ")):
			fmt.Fprintf(&output, "+++ b/%s\n", driftline.GitIgnorePath)
		case bytes.HasPrefix(line, []byte("Binary files ")):
			if targetMissing {
				fmt.Fprintf(&output, "Binary files /dev/null and b/%s differ\n", driftline.GitIgnorePath)
			} else {
				fmt.Fprintf(&output, "Binary files a/%s and b/%s differ\n", driftline.GitIgnorePath, driftline.GitIgnorePath)
			}
		default:
			output.Write(line)
		}
	}
	return output.Bytes()
}

func printGitDiff(w io.Writer, sourcePath, targetPath string, targetMissing bool) error {
	left := targetPath
	if targetMissing {
		left = os.DevNull
	}
	cmd := exec.Command("git", "diff", "--no-index", "--", left, sourcePath)
	data, err := cmd.CombinedOutput()
	if len(data) > 0 {
		fmt.Fprint(w, string(data))
	}
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return fmt.Errorf("run git diff: %w", err)
}
