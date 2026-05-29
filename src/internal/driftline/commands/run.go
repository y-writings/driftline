package commands

import (
	"flag"
	"fmt"
	"io"

	"github.com/y-writings/driftline/src/internal/driftline"
)

func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 { printUsage(stderr); return fmt.Errorf("command is required") }
	cmd := args[0]
	opts, rest, err := parseOptions(args[1:], stderr)
	if err != nil { return err }
	if len(rest) > 0 { return fmt.Errorf("unexpected arguments: %v", rest) }
	switch cmd {
	case "pull":
		return runPull(opts, stdout)
	case "update":
		return runUpdate(opts, stdout)
	case "help", "-h", "--help":
		printUsage(stdout); return nil
	default:
		printUsage(stderr); return fmt.Errorf("unknown command %q", cmd)
	}
}

func parseOptions(args []string, stderr io.Writer) (driftline.Options, []string, error) {
	opts := driftline.DefaultOptions()
	fs := flag.NewFlagSet("driftline", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.ManifestPath, "manifest", opts.ManifestPath, "pull config path")
	fs.StringVar(&opts.LockPath, "lock", opts.LockPath, "lock file path")
	fs.StringVar(&opts.TargetDir, "target-dir", opts.TargetDir, "target directory")
	if err := fs.Parse(args); err != nil { return opts, nil, err }
	return opts, fs.Args(), nil
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: driftline <pull|update> [options]")
}
