package main

import (
	"fmt"
	"os"

	"github.com/y-writings/templates/src/internal/templatesync/commands"
)

func main() {
	if err := commands.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
