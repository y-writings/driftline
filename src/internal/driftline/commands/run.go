package commands

import (
	"errors"
	"fmt"
	"io"

	"github.com/y-writings/driftline/src/internal/driftline"
)

var errDrift = errors.New("drift detected")

type Runner struct {
	Source driftline.SourceClient
}

func Run(args []string, stdout, stderr io.Writer) error {
	return Runner{Source: driftline.NewGitHubClientFromEnv()}.Run(args, stdout, stderr)
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) error {
	if r.Source == nil {
		r.Source = driftline.NewGitHubClientFromEnv()
	}
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("command is required")
	}

	switch args[0] {
	case "init":
		opts, err := parseInitOptions(args[1:])
		if err != nil {
			return err
		}
		return runInit(r.Source, opts, stdout)
	case "check":
		opts, err := parseTargetOptions(args[1:])
		if err != nil {
			return err
		}
		return runCheck(r.Source, opts, stdout)
	case "diff":
		opts, err := parseTargetOptions(args[1:])
		if err != nil {
			return err
		}
		return runDiff(r.Source, opts, stdout)
	case "update":
		opts, err := parseTargetOptions(args[1:])
		if err != nil {
			return err
		}
		return runUpdate(r.Source, opts, stdout)
	case "prune":
		opts, err := parseTargetOptions(args[1:])
		if err != nil {
			return err
		}
		return runPrune(r.Source, opts, stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

type TargetOptions struct {
	TargetDir string
}

type InitOptions struct {
	Repository string
	Ref        string
	TargetDir  string
}

func parseTargetOptions(args []string) (TargetOptions, error) {
	opts := TargetOptions{TargetDir: "."}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--target-dir":
			if i+1 >= len(args) {
				return opts, errors.New("--target-dir requires a value")
			}
			opts.TargetDir = args[i+1]
			i++
		default:
			return opts, fmt.Errorf("unknown option %q", args[i])
		}
	}
	return opts, nil
}

func parseInitOptions(args []string) (InitOptions, error) {
	opts := InitOptions{TargetDir: "."}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--ref":
			if i+1 >= len(args) {
				return opts, errors.New("--ref requires a value")
			}
			opts.Ref = args[i+1]
			i++
		case "--target-dir":
			if i+1 >= len(args) {
				return opts, errors.New("--target-dir requires a value")
			}
			opts.TargetDir = args[i+1]
			i++
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return opts, fmt.Errorf("unknown option %q", args[i])
			}
			if opts.Repository != "" {
				return opts, fmt.Errorf("unexpected argument %q", args[i])
			}
			opts.Repository = args[i]
		}
	}
	if opts.Repository == "" {
		return opts, errors.New("repository is required")
	}
	return opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: driftline <command> [options]

commands:
  init owner/repo  create driftline.yaml from a GitHub Source Repository
  check            check whether target files match the Source Repository
  diff             show diffs for files that would be added or updated
  update           copy added/updated files and refresh driftline-lock.yaml
  prune            remove stale files when they are unchanged locally

examples:
  driftline init owner/repo
  driftline init owner/repo --ref main --target-dir .
  driftline check --target-dir .

options:
  --target-dir string  target repository directory (default ".")
  --ref string         init-only ref to preserve in driftline.yaml

authentication:
  set GITHUB_TOKEN for private repositories or higher rate limits`)
}
