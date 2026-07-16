//go:build !windows

package main

import "github.com/urfave/cli/v3"

func platformCommands() []*cli.Command {
	return nil
}
