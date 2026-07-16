package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// newFlagSet returns a FlagSet whose -h/--help output uses long --flag form.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() { printFlagUsage(fs, osStderr()) }
	return fs
}

func osStderr() io.Writer {
	return os.Stderr
}

func printFlagUsage(fs *flag.FlagSet, w io.Writer) {
	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		if f.DefValue == "" {
			fmt.Fprintf(w, "  %s\n    %s\n", name, f.Usage)
			return
		}
		fmt.Fprintf(w, "  %s\n    %s (default %q)\n", name, f.Usage, f.DefValue)
	})
}

func printCommandHelp(title, usage string, fs *flag.FlagSet) {
	fmt.Fprintf(osStderr(), "%s\n\nUsage:\n%s\n", title, usage)
	if fs != nil {
		hasFlags := false
		fs.VisitAll(func(*flag.Flag) { hasFlags = true })
		if hasFlags {
			fmt.Fprintln(osStderr(), "Flags:")
			printFlagUsage(fs, osStderr())
		}
	}
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "-h", "--help", "help":
			return true
		}
	}
	return false
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(normalizeArgs(args))
}

// normalizeArgs lets users pass either -flag or --flag (Go flag accepts both).
func normalizeArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") && !strings.HasPrefix(arg, "---") {
			out = append(out, "-"+strings.TrimPrefix(arg, "--"))
			continue
		}
		out = append(out, arg)
	}
	return out
}
