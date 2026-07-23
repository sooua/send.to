package main

import (
	"strings"

	"github.com/urfave/cli/v2"
)

// reorderArgs moves option flags ahead of positional arguments.
//
// urfave/cli parses with the standard library's flag package, which stops
// looking for flags at the first non-flag argument. So `send put a.txt
// --max-downloads 1` silently uploads with no download limit at all — the
// user asked for a one-time link and got a permanent one. Since that is
// exactly how people type commands, rewrite the argument list instead of
// documenting the trap.
//
// Everything after a literal `--` is left alone, and `-` on its own stays a
// positional argument because it means stdin.
func reorderArgs(app *cli.App, args []string) []string {
	if len(args) < 2 {
		return args
	}

	out := []string{args[0]}
	rest := args[1:]

	// Walk the command chain so the flag set in scope is the right one.
	flags := append([]cli.Flag{}, app.Flags...)
	commands := app.Commands

	i := 0
	for i < len(rest) {
		if strings.HasPrefix(rest[i], "-") {
			break
		}

		cmd := findCommand(commands, rest[i])
		if cmd == nil {
			break
		}

		out = append(out, rest[i])
		flags = append(flags, cmd.Flags...)
		commands = cmd.Subcommands
		i++
	}

	// Which flags consume the following argument as their value.
	takesValue := map[string]bool{}
	for _, f := range flags {
		_, isBool := f.(*cli.BoolFlag)
		for _, name := range f.Names() {
			takesValue[name] = !isBool
		}
	}

	var opts, positional []string

	for ; i < len(rest); i++ {
		arg := rest[i]

		switch {
		case arg == "--":
			positional = append(positional, rest[i+1:]...)
			i = len(rest)

		case arg == "-" || !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)

		default:
			opts = append(opts, arg)

			name := strings.TrimLeft(arg, "-")
			if strings.ContainsRune(name, '=') {
				break // --flag=value carries its own value
			}
			if takesValue[name] && i+1 < len(rest) {
				i++
				opts = append(opts, rest[i])
			}
		}
	}

	out = append(out, opts...)
	if len(positional) > 0 {
		out = append(out, "--")
		out = append(out, positional...)
	}

	return out
}

func findCommand(commands []*cli.Command, name string) *cli.Command {
	for _, cmd := range commands {
		if cmd.Name == name {
			return cmd
		}
		for _, alias := range cmd.Aliases {
			if alias == name {
				return cmd
			}
		}
	}
	return nil
}
