package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"rsc.io/getopt"
)

var unfocusedSymbol = flag.String("unfocused", "⋅", "Symbol for unfocused columns")
var focusedSymbol = flag.String("focused", "⊙", "Symbol for focused columns")
var unfocusedFloatingSymbol = flag.String("unfocused-floating", "∗", "Symbol for unfocused floating windows")
var focusedFloatingSymbol = flag.String("focused-floating", "⊛", "Symbol for focused floating windows")
var outputName = flag.String("output", "", "The output (DP-1, HDMI-1, etc.) that this indicator is for")

type boolFlag interface {
	IsBoolFlag() bool
}

func init() {
	getopt.CommandLine.Init("waybar-niri-windows", flag.ContinueOnError)
	getopt.CommandLine.SetOutput(io.Discard)
	getopt.Alias("u", "unfocused")
	getopt.Alias("f", "focused")
	getopt.Alias("U", "unfocused-floating")
	getopt.Alias("F", "focused-floating")
	getopt.Alias("o", "output")
	getopt.CommandLine.Usage = func() {}
}

func parseFlags(f *getopt.FlagSet, args []string) error {
	for len(args) > 0 {
		arg := args[0]
		if len(arg) < 2 || arg[0] != '-' {
			break
		}
		args = args[1:]
		if arg[:2] == "--" {
			// Process single long option.
			if arg == "--" {
				break
			}
			name := arg[2:]
			value := ""
			haveValue := false
			if i := strings.Index(name, "="); i >= 0 {
				name, value = name[:i], name[i+1:]
				haveValue = true
			}
			fg := f.Lookup(name)
			if fg == nil {
				if name == "h" || name == "help" {
					return flag.ErrHelp
				}
				return fmt.Errorf("flag provided but not defined: --%s", name)
			}
			if b, ok := fg.Value.(boolFlag); ok && b.IsBoolFlag() {
				if haveValue {
					if err := fg.Value.Set(value); err != nil {
						return fmt.Errorf("invalid boolean value %q for --%s: %v", value, name, err)
					}
				} else {
					if err := fg.Value.Set("true"); err != nil {
						return fmt.Errorf("invalid boolean flag %s: %v", name, err)
					}
				}
				continue
			}
			if !haveValue {
				if len(args) == 0 {
					return fmt.Errorf("missing argument for --%s", name)
				}
				value, args = args[0], args[1:]
			}
			if err := fg.Value.Set(value); err != nil {
				return fmt.Errorf("invalid value %q for flag --%s: %v", value, name, err)
			}
			continue
		}

		// Process one or more short options.
		for arg = arg[1:]; arg != ""; {
			r, size := utf8.DecodeRuneInString(arg)
			if r == utf8.RuneError && size == 1 {
				return fmt.Errorf("invalid UTF8 in command-line flags")
			}
			name := arg[:size]
			arg = arg[size:]
			fg := f.Lookup(name)
			if fg == nil {
				if name == "h" {
					return flag.ErrHelp
				}
				return fmt.Errorf("flag provided but not defined: -%s", name)
			}
			if b, ok := fg.Value.(boolFlag); ok && b.IsBoolFlag() {
				if err := fg.Value.Set("true"); err != nil {
					return fmt.Errorf("invalid boolean flag %s: %v", name, err)
				}
				continue
			}
			if arg == "" {
				if len(args) == 0 {
					return fmt.Errorf("missing argument for -%s", name)
				}
				arg, args = args[0], args[1:]
			}
			if err := fg.Value.Set(arg); err != nil {
				return fmt.Errorf("invalid value %q for flag -%s: %v", arg, name, err)
			}
			break // consumed arg
		}
	}

	// Arrange for flag.NArg, flag.Args, etc to work properly.
	f.FlagSet.Parse(append([]string{"--"}, args...))
	return nil
}
