package main

import (
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 0
	}
	switch args[0] {
	case "serve":
		return serveCmd(args[1:])
	case "init":
		return initCmd(args[1:])
	case "catalog":
		return catalogCmd(args[1:])
	case "credential":
		return credentialCmd(args[1:])
	case "keys":
		return keysCmd(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "cincai: unknown command %q\n", args[0])
		printUsage()
		return 2
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `cincai — AI gateway (chat + media)

Usage:
  cincai init [--dir PATH] [--force]
  cincai serve [--config PATH]
  cincai catalog validate [--config PATH] [--catalog PATH]
  cincai credential <subcommand> [flags]
  cincai keys <subcommand> [flags]

Run "cincai <command> --help" for command flags.

`)
}
