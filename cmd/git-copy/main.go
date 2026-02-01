package main

import (
	"fmt"
	"os"

	"github.com/obinnaokechukwu/git-copy/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
