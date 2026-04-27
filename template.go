package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config holds runtime options for template processing.
type Config struct {
	Backup  bool
	DryRun  bool
	Strict  bool
	Prefix  string
	EnvFile string
}

// Stats tracks processing metrics.
type Stats struct {
	Files    int
	Expanded int
	Defaults int
	Skipped  int
}

// varPattern matches ${KEY} and ${KEY:-default} with optional leading backslashes.
//
//	Group 1: leading backslashes (may be empty)
//	Group 2: variable name
//	Group 3: ":-" separator (empty when no default is specified)
//	Group 4: default value (empty string is valid when separator is present)
var varPattern = regexp.MustCompile(`(\\*)\$\{(.+?)(?:(:-)(.*?))?\}`)

// apply processes all files matching the given glob patterns.
func apply(cfg Config, globs []string) (Stats, error) {
	var st Stats

	env := loadEnv()

	if cfg.EnvFile != "" {
		extra, err := loadEnvFile(cfg.EnvFile)
		if err != nil {
			return st, fmt.Errorf("load env file: %w", err)
		}
		for k, v := range extra {
			env[k] = v
		}
	}

	matched := false
	for _, pattern := range globs {
		files, err := filepath.Glob(pattern)
		if err != nil {
			return st, fmt.Errorf("invalid glob %q: %w", pattern, err)
		}
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil || info.IsDir() {
				continue
			}
			matched = true
			if err := processFile(cfg, file, env, &st); err != nil {
				return st, err
			}
		}
	}

	if !matched {
		return st, fmt.Errorf("no files matched globs %v", globs)
	}
	return st, nil
}

// processFile reads a file, expands variable references, and writes the result.
func processFile(cfg Config, file string, env map[string]string, st *Stats) error {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	log.Printf("Processing %s", file)
	st.Files++

	parsed, err := expand(string(content), file, env, cfg, st)
	if err != nil {
		return err
	}

	if cfg.DryRun {
		fmt.Printf("--- %s\n%s", file, parsed)
		return nil
	}

	if cfg.Backup {
		if err := createBackup(file); err != nil {
			return fmt.Errorf("backup %s: %w", file, err)
		}
		log.Printf("Created backup %s.bak", file)
	}

	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	return os.WriteFile(file, []byte(parsed), info.Mode())
}

// expand replaces all ${KEY} and ${KEY:-default} references in content with
// values from env. Backslash escaping follows even/odd rules:
//
//	\${VAR}    → ${VAR}          (1 backslash = escaped)
//	\\${VAR}   → \<value>        (2 backslashes = not escaped)
//	\\\${VAR}  → \${VAR}         (3 backslashes = escaped)
//	\\\\${VAR} → \\<value>       (4 backslashes = not escaped)
func expand(content, file string, env map[string]string, cfg Config, st *Stats) (string, error) {
	var firstErr error

	result := varPattern.ReplaceAllStringFunc(content, func(match string) string {
		if firstErr != nil {
			return match
		}

		groups := varPattern.FindStringSubmatch(match)
		backslashes := groups[1]
		key := groups[2]
		sep := groups[3]
		def := groups[4]
		n := len(backslashes)

		// Odd backslashes: escape — strip one layer and keep the ${...} literal.
		if n%2 == 1 {
			return strings.Repeat(`\`, (n-1)/2) + match[n:]
		}

		prefix := strings.Repeat(`\`, n/2)

		// Prefix filter: skip variables that don't match the required prefix.
		if cfg.Prefix != "" && !strings.HasPrefix(key, cfg.Prefix) {
			return match
		}

		value, ok := env[key]
		if !ok {
			if sep == "" {
				st.Skipped++
				if cfg.Strict {
					firstErr = fmt.Errorf("%s: required variable %q is not set", file, key)
				} else {
					log.Printf("Skipping unset variable %q in %s", key, file)
				}
				return match
			}
			if cfg.Strict {
				firstErr = fmt.Errorf("%s: variable %q is not set, strict mode forbids default %q", file, key, def)
				return match
			}
			st.Defaults++
			log.Printf("Using default %q for %q in %s", def, key, file)
			value = def
		} else {
			st.Expanded++
			log.Printf("Expanding %q in %s", key, file)
		}

		return prefix + value
	})

	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

// createBackup copies file to file.bak preserving permissions.
func createBackup(file string) error {
	src, err := os.Open(file)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}

	dst, err := os.OpenFile(file+".bak", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}

	if _, err = io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// loadEnv parses os.Environ() into a map.
func loadEnv() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		if k, v, ok := strings.Cut(entry, "="); ok {
			env[k] = v
		}
	}
	return env
}

// loadEnvFile reads a .env file and returns key-value pairs.
// Supports KEY=VALUE, export KEY=VALUE, comments (#), empty lines, and quoted values.
func loadEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		k = strings.TrimPrefix(k, "export ")
		v = strings.TrimSpace(v)
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		env[k] = v
	}
	return env, nil
}
