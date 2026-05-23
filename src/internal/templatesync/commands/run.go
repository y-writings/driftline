package commands

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/y-writings/templates/src/internal/templatesync"
)

var errDrift = errors.New("template drift detected")

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		printUsage(stderr)
		return errors.New("command is required")
	}
	command := args[0]
	opts, rest, err := parseOptions(args[1:], stderr)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("unexpected arguments: %v", rest)
	}

	switch command {
	case "check":
		return runCheck(opts, stdout)
	case "diff":
		return runDiff(opts, stdout)
	case "update":
		return runUpdate(opts, stdout)
	case "prune":
		return runPrune(opts, stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command %q", command)
	}
}

func parseOptions(args []string, stderr io.Writer) (templatesync.Options, []string, error) {
	opts := templatesync.DefaultOptions()
	fs := flag.NewFlagSet("template-sync", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.ManifestPath, "manifest", opts.ManifestPath, "manifest path relative to template dir")
	fs.StringVar(&opts.LockPath, "lock", opts.LockPath, "lock file path relative to target dir")
	fs.StringVar(&opts.TemplateDir, "template-dir", opts.TemplateDir, "template repository directory")
	fs.StringVar(&opts.TargetDir, "target-dir", opts.TargetDir, "target repository directory")
	fs.StringVar(&opts.Repository, "repository", opts.Repository, "repository value written to lock file")
	fs.StringVar(&opts.Ref, "ref", opts.Ref, "ref value written to lock file")
	if err := fs.Parse(args); err != nil {
		return opts, nil, err
	}
	return opts, fs.Args(), nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: template-sync <command> [options]

commands:
  check   check whether target files match the template
  diff    show diffs for files that would be added or updated
  update  copy added/updated files and refresh the lock file
  prune   remove manifest-deleted files when they are unchanged locally

options:
  --template-dir string  template repository directory (default ".")
  --target-dir string    target repository directory (default ".")
  --manifest string      manifest path relative to template dir (default "templates.yaml")
  --lock string          lock file path relative to target dir (default ".template-sync.lock")
  --repository string    repository value written to lock file
  --ref string           ref value written to lock file`)
}
