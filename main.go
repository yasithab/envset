package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"syscall"

	"github.com/spf13/cobra"
)

var Version = "dev"

func main() {
	args, execArgs := splitOnDoubleDash(os.Args[1:])
	os.Args = append([]string{os.Args[0]}, args...)

	var (
		backup  bool
		dryRun  bool
		strict  bool
		verbose bool
		prefix  string
		envFile string
	)

	root := &cobra.Command{
		Use:          "envset [flags] <glob> [glob...]",
		Short:        "Set environment variables into configuration files",
		Version:      Version,
		SilenceUsage: true,
		Args:         cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !verbose {
				log.SetOutput(io.Discard)
			}

			cfg := Config{
				Backup:  backup,
				DryRun:  dryRun,
				Strict:  strict,
				Prefix:  prefix,
				EnvFile: envFile,
			}

			st, err := apply(cfg, args)
			if err != nil {
				return err
			}

			log.Printf("Done: %d file(s), %d expanded, %d defaults, %d skipped",
				st.Files, st.Expanded, st.Defaults, st.Skipped)

			if !dryRun && len(execArgs) > 0 {
				if execErr := syscall.Exec(execArgs[0], execArgs, os.Environ()); execErr != nil {
					return fmt.Errorf("exec %q: %w", execArgs[0], execErr)
				}
			}
			return nil
		},
	}

	root.Flags().BoolVarP(&backup, "backup", "b", false, "Create .bak backup before modifying")
	root.Flags().BoolVarP(&dryRun, "dry-run", "d", false, "Output to stdout instead of inline replacement")
	root.Flags().BoolVarP(&strict, "strict", "s", false, "Fail on any unset variable (with or without defaults)")
	root.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose logging")
	root.Flags().StringVarP(&prefix, "prefix", "p", "", "Only expand variables matching this prefix (e.g. EP_)")
	root.Flags().StringVarP(&envFile, "env-file", "e", "", "Load additional env vars from a file before processing")

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func splitOnDoubleDash(args []string) (before, after []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}
