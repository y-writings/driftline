package commands

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/y-writings/driftline/src/internal/driftline"
	"github.com/y-writings/driftline/src/internal/driftline/github"
)

var errDrift = errors.New("drift detected")

type Runner struct {
	Source driftline.SourceClient
}

func Run(args []string, stdout, stderr io.Writer) error {
	return Runner{Source: github.NewClientFromEnv()}.Run(args, stdout, stderr)
}

func (r Runner) Run(args []string, stdout, stderr io.Writer) error {
	if r.Source == nil {
		r.Source = github.NewClientFromEnv()
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
		opts, err := parseUpdateOptions(args[1:])
		if err != nil {
			return err
		}
		return runUpdate(r.Source, opts, stdout)
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

type UpdateOptions struct {
	TargetDir string
	ForceKey  string
}

type InitOptions struct {
	Repository string
	Ref        string
	TargetDir  string
	Force      bool
}

func parseTargetOptions(args []string) (TargetOptions, error) {
	opts := TargetOptions{TargetDir: "."}
	for i := 0; i < len(args); i++ {
		if value, ok, err := parseStringOption(args, &i, "target-dir"); err != nil {
			return opts, err
		} else if ok {
			opts.TargetDir = value
			continue
		}
		return opts, fmt.Errorf("unknown option %q", args[i])
	}
	return opts, nil
}

func parseUpdateOptions(args []string) (UpdateOptions, error) {
	opts := UpdateOptions{TargetDir: "."}
	for i := 0; i < len(args); i++ {
		if value, ok, err := parseStringOption(args, &i, "target-dir"); err != nil {
			return opts, err
		} else if ok {
			opts.TargetDir = value
			continue
		}
		if value, ok, err := parseStringOption(args, &i, "force"); err != nil {
			return opts, err
		} else if ok {
			if value == "" {
				return opts, fmt.Errorf("--force requires a value")
			}
			if opts.ForceKey != "" {
				return opts, fmt.Errorf("--force may be provided once")
			}
			opts.ForceKey = value
			continue
		}
		return opts, fmt.Errorf("unknown option %q", args[i])
	}
	return opts, nil
}

func parseInitOptions(args []string) (InitOptions, error) {
	opts := InitOptions{TargetDir: "."}
	for i := 0; i < len(args); i++ {
		if value, ok, err := parseStringOption(args, &i, "ref"); err != nil {
			return opts, err
		} else if ok {
			opts.Ref = value
			continue
		}
		if value, ok, err := parseStringOption(args, &i, "target-dir"); err != nil {
			return opts, err
		} else if ok {
			opts.TargetDir = value
			continue
		}
		if ok, err := parseBoolOption(args, &i, "force"); err != nil {
			return opts, err
		} else if ok {
			if opts.Force {
				return opts, fmt.Errorf("--force may be provided once")
			}
			opts.Force = true
			continue
		}
		if len(args[i]) > 0 && args[i][0] == '-' {
			return opts, fmt.Errorf("unknown option %q", args[i])
		}
		if opts.Repository != "" {
			return opts, fmt.Errorf("unexpected argument %q", args[i])
		}
		opts.Repository = args[i]
	}
	if opts.Repository == "" {
		return opts, errors.New("repository is required")
	}
	return opts, nil
}

func parseStringOption(args []string, index *int, name string) (string, bool, error) {
	arg := args[*index]
	for _, prefix := range []string{"--" + name, "-" + name} {
		if arg == prefix {
			if *index+1 >= len(args) {
				return "", true, fmt.Errorf("%s requires a value", prefix)
			}
			*index = *index + 1
			return args[*index], true, nil
		}
		if value, ok := strings.CutPrefix(arg, prefix+"="); ok {
			return value, true, nil
		}
	}
	return "", false, nil
}

func parseBoolOption(args []string, index *int, name string) (bool, error) {
	arg := args[*index]
	for _, prefix := range []string{"--" + name, "-" + name} {
		if arg == prefix {
			return true, nil
		}
		if strings.HasPrefix(arg, prefix+"=") {
			return true, fmt.Errorf("%s does not accept a value", prefix)
		}
	}
	return false, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: driftline <command> [options]

commands:
  init owner/repo  create .driftline/sync.toml from a GitHub Source Repository
  check            check Target Repository state against the Contract
  diff             show planned content changes
  update           reconcile Managed files, Gitignore section, and Sync manifest

examples:
  driftline init owner/repo
  driftline init owner/repo --force
  driftline init owner/repo --ref main --target-dir .
  driftline check --target-dir .
  driftline update --force github-workflow.ci

options:
  --target-dir string  target repository directory (default ".")
  --ref string         init-only ref to preserve in .driftline/sync.toml
  --force              init-only adopt existing regular Managed target files
  --force group.file   update-only one-time conflict overwrite

authentication:
  set GITHUB_TOKEN for private repositories or higher rate limits`)
}
