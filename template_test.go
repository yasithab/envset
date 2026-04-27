package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	os.Setenv("DATABASE", "db.example.com")
	os.Setenv("NULL", "")
	os.Setenv("MODE", "debug")
	os.Setenv("ES_HOST", "api.example.com")
	os.Setenv("ES_PORT", "8080")
	os.Exit(m.Run())
}

// ── expand ───────────────────────────────────────────────────────────────────

func TestExpand(t *testing.T) {
	env := loadEnv()
	cfg := Config{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "host=${DATABASE}", "host=db.example.com"},
		{"empty value", "val=${NULL}", "val="},
		{"multiple", "${DATABASE} ${MODE}", "db.example.com debug"},
		{"not a var", "$NOT_A_VARIABLE", "$NOT_A_VARIABLE"},
		{"mixed text", "begin ${DATABASE} end", "begin db.example.com end"},
		{"adjacent", "${DATABASE}${MODE}", "db.example.comdebug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st Stats
			got, err := expand(tt.input, "test", env, cfg, &st)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandDefaults(t *testing.T) {
	env := loadEnv()
	cfg := Config{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"default used", "${UNDEFINED:-fallback}", "fallback"},
		{"default not used", "${DATABASE:-fallback}", "db.example.com"},
		{"empty default", "${UNDEFINED:-}", ""},
		{"default with url", "${UNDEFINED:-http://example.com/path?a=b}", "http://example.com/path?a=b"},
		{"default negative number", "${UNDEFINED:--1}", "-1"},
		{"double default", "${UNDEFINED:-a} ${UNDEFINED:-b}", "a b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st Stats
			got, err := expand(tt.input, "test", env, cfg, &st)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandEscape(t *testing.T) {
	env := loadEnv()
	cfg := Config{}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"1 backslash", `\${DATABASE}`, `${DATABASE}`},
		{"2 backslashes", `\\${DATABASE}`, `\db.example.com`},
		{"3 backslashes", `\\\${DATABASE}`, `\${DATABASE}`},
		{"4 backslashes", `\\\\${DATABASE}`, `\\db.example.com`},
		{"1 backslash with default", `\${DATABASE:-fallback}`, `${DATABASE:-fallback}`},
		{"3 backslashes with default", `\\\${UNDEFINED:-val}`, `\${UNDEFINED:-val}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var st Stats
			got, err := expand(tt.input, "test", env, cfg, &st)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExpandUnsetVarSkipped(t *testing.T) {
	env := loadEnv()
	cfg := Config{}

	t.Run("unset var left as-is", func(t *testing.T) {
		var st Stats
		got, err := expand("${UNDEFINED}", "test.conf", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "${UNDEFINED}" {
			t.Errorf("got %q, want %q", got, "${UNDEFINED}")
		}
		if st.Skipped != 1 {
			t.Errorf("skipped: got %d, want 1", st.Skipped)
		}
	})

	t.Run("mixed set and unset", func(t *testing.T) {
		var st Stats
		got, err := expand("db=${DATABASE} name=${name}", "test.conf", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "db=db.example.com name=${name}"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if st.Expanded != 1 || st.Skipped != 1 {
			t.Errorf("stats: expanded=%d skipped=%d, want 1/1", st.Expanded, st.Skipped)
		}
	})

	t.Run("node template safe", func(t *testing.T) {
		var st Stats
		input := "const url = `https://${host}:${port}/api`;\nDB=${DATABASE}"
		got, err := expand(input, "app.js", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "const url = `https://${host}:${port}/api`;\nDB=db.example.com"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestExpandErrors(t *testing.T) {
	env := loadEnv()

	t.Run("strict missing var", func(t *testing.T) {
		var st Stats
		_, err := expand("${UNDEFINED}", "test.conf", env, Config{Strict: true}, &st)
		if err == nil {
			t.Fatal("expected error in strict mode for unset variable")
		}
	})

	t.Run("strict with default", func(t *testing.T) {
		var st Stats
		_, err := expand("${UNDEFINED:-fallback}", "test.conf", env, Config{Strict: true}, &st)
		if err == nil {
			t.Fatal("expected error in strict mode with default")
		}
	})

	t.Run("strict allows set vars", func(t *testing.T) {
		var st Stats
		got, err := expand("${DATABASE}", "test.conf", env, Config{Strict: true}, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "db.example.com" {
			t.Errorf("got %q, want %q", got, "db.example.com")
		}
	})
}

// ── prefix ───────────────────────────────────────────────────────────────────

func TestExpandPrefix(t *testing.T) {
	env := loadEnv()
	cfg := Config{Prefix: "ES_"}

	t.Run("matching prefix expanded", func(t *testing.T) {
		var st Stats
		got, err := expand("host=${ES_HOST}:${ES_PORT}", "test", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "host=api.example.com:8080" {
			t.Errorf("got %q", got)
		}
		if st.Expanded != 2 {
			t.Errorf("expanded: got %d, want 2", st.Expanded)
		}
	})

	t.Run("non-matching prefix left as-is", func(t *testing.T) {
		var st Stats
		got, err := expand("${DATABASE} ${ES_HOST}", "test", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "${DATABASE} api.example.com" {
			t.Errorf("got %q", got)
		}
		if st.Expanded != 1 {
			t.Errorf("expanded: got %d, want 1", st.Expanded)
		}
	})

	t.Run("prefix with node template", func(t *testing.T) {
		var st Stats
		input := "url=https://${host}:${port}\napi=${ES_HOST}"
		got, err := expand(input, "test", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "url=https://${host}:${port}\napi=api.example.com"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("prefix strict only checks matching", func(t *testing.T) {
		var st Stats
		cfg := Config{Prefix: "ES_", Strict: true}
		// ${DATABASE} doesn't match prefix, so strict doesn't apply to it
		got, err := expand("${DATABASE} ${ES_HOST}", "test", env, cfg, &st)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "${DATABASE} api.example.com" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("prefix strict fails on unset matching var", func(t *testing.T) {
		var st Stats
		cfg := Config{Prefix: "ES_", Strict: true}
		_, err := expand("${ES_MISSING}", "test", env, cfg, &st)
		if err == nil {
			t.Fatal("expected error for unset variable matching prefix in strict mode")
		}
	})
}

// ── env file ─────────────────────────────────────────────────────────────────

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, ".env")

	content := `# Database config
DB_HOST=localhost
DB_PORT=5432

# Quoted values
DB_NAME="mydb"
DB_PASS='s3cret'

# Spaces around =
API_KEY = abc123

# Export prefix
export EXPORTED_VAR=exported_value
`
	os.WriteFile(file, []byte(content), 0o644)

	env, err := loadEnvFile(file)
	if err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}

	tests := []struct {
		key, want string
	}{
		{"DB_HOST", "localhost"},
		{"DB_PORT", "5432"},
		{"DB_NAME", "mydb"},
		{"DB_PASS", "s3cret"},
		{"API_KEY", "abc123"},
		{"EXPORTED_VAR", "exported_value"},
	}

	for _, tt := range tests {
		if got := env[tt.key]; got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.key, got, tt.want)
		}
	}

	if _, ok := env["# Database config"]; ok {
		t.Error("comment line parsed as key")
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	_, err := loadEnvFile("/nonexistent/.env")
	if err == nil {
		t.Fatal("expected error for missing env file")
	}
}

func TestApplyWithEnvFile(t *testing.T) {
	dir := t.TempDir()

	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("CUSTOM_VAR=from-envfile\n"), 0o644)

	confFile := filepath.Join(dir, "app.conf")
	os.WriteFile(confFile, []byte("val=${CUSTOM_VAR}"), 0o644)

	cfg := Config{EnvFile: envFile}
	st, err := apply(cfg, []string{filepath.Join(dir, "*.conf")})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	got, _ := os.ReadFile(confFile)
	if string(got) != "val=from-envfile" {
		t.Errorf("got %q", string(got))
	}
	if st.Expanded != 1 {
		t.Errorf("expanded: got %d, want 1", st.Expanded)
	}
}

func TestEnvFileOverridesSystemEnv(t *testing.T) {
	dir := t.TempDir()

	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("DATABASE=overridden.example.com\n"), 0o644)

	confFile := filepath.Join(dir, "app.conf")
	os.WriteFile(confFile, []byte("${DATABASE}"), 0o644)

	cfg := Config{EnvFile: envFile}
	apply(cfg, []string{filepath.Join(dir, "*.conf")})

	got, _ := os.ReadFile(confFile)
	if string(got) != "overridden.example.com" {
		t.Errorf("got %q, want env file value to override", string(got))
	}
}

// ── stats ────────────────────────────────────────────────────────────────────

func TestStats(t *testing.T) {
	dir := t.TempDir()

	f1 := filepath.Join(dir, "a.conf")
	f2 := filepath.Join(dir, "b.conf")
	os.WriteFile(f1, []byte("${DATABASE} ${UNDEFINED:-fallback} ${MISSING}"), 0o644)
	os.WriteFile(f2, []byte("${MODE}"), 0o644)

	cfg := Config{}
	st, err := apply(cfg, []string{filepath.Join(dir, "*.conf")})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if st.Files != 2 {
		t.Errorf("files: got %d, want 2", st.Files)
	}
	if st.Expanded != 2 {
		t.Errorf("expanded: got %d, want 2", st.Expanded)
	}
	if st.Defaults != 1 {
		t.Errorf("defaults: got %d, want 1", st.Defaults)
	}
	if st.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", st.Skipped)
	}
}

// ── processFile ──────────────────────────────────────────────────────────────

func TestProcessFile(t *testing.T) {
	env := loadEnv()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.conf")

	content := "host=${DATABASE}\nmode=${MODE}\n"
	os.WriteFile(file, []byte(content), 0o644)

	var st Stats
	cfg := Config{}
	if err := processFile(cfg, file, env, &st); err != nil {
		t.Fatalf("processFile: %v", err)
	}

	got, _ := os.ReadFile(file)
	want := "host=db.example.com\nmode=debug\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}

func TestProcessFilePreservesPermissions(t *testing.T) {
	env := loadEnv()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.conf")

	os.WriteFile(file, []byte("${DATABASE}"), 0o755)

	var st Stats
	cfg := Config{}
	if err := processFile(cfg, file, env, &st); err != nil {
		t.Fatal(err)
	}

	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0o755 {
		t.Errorf("permissions changed: got %v, want %v", info.Mode().Perm(), os.FileMode(0o755))
	}
}

func TestProcessFileDryRun(t *testing.T) {
	env := loadEnv()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.conf")

	original := "host=${DATABASE}\n"
	os.WriteFile(file, []byte(original), 0o644)

	var st Stats
	cfg := Config{DryRun: true}

	r, w, _ := os.Pipe()
	oldStdout := os.Stdout
	os.Stdout = w

	if err := processFile(cfg, file, env, &st); err != nil {
		os.Stdout = oldStdout
		t.Fatalf("processFile: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.HasPrefix(output, "--- ") {
		t.Errorf("stdout missing file header, got %q", output)
	}
	if !strings.HasSuffix(output, "host=db.example.com\n") {
		t.Errorf("stdout got %q", output)
	}

	got, _ := os.ReadFile(file)
	if string(got) != original {
		t.Error("file was modified in dry-run")
	}
}

func TestProcessFileBackup(t *testing.T) {
	env := loadEnv()
	dir := t.TempDir()
	file := filepath.Join(dir, "test.conf")

	original := "host=${DATABASE}\n"
	os.WriteFile(file, []byte(original), 0o644)

	var st Stats
	cfg := Config{Backup: true}
	if err := processFile(cfg, file, env, &st); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(file + ".bak")
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(backup) != original {
		t.Errorf("backup content: got %q", string(backup))
	}

	got, _ := os.ReadFile(file)
	if string(got) != "host=db.example.com\n" {
		t.Errorf("file content: got %q", string(got))
	}
}

// ── apply ────────────────────────────────────────────────────────────────────

func TestApply(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.conf")
	f2 := filepath.Join(dir, "b.conf")

	os.WriteFile(f1, []byte("${DATABASE}"), 0o644)
	os.WriteFile(f2, []byte("${MODE}"), 0o644)

	cfg := Config{}
	if _, err := apply(cfg, []string{filepath.Join(dir, "*.conf")}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	got1, _ := os.ReadFile(f1)
	got2, _ := os.ReadFile(f2)
	if string(got1) != "db.example.com" {
		t.Errorf("a.conf: got %q", string(got1))
	}
	if string(got2) != "debug" {
		t.Errorf("b.conf: got %q", string(got2))
	}
}

func TestApplyNoMatches(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{}
	_, err := apply(cfg, []string{filepath.Join(dir, "*.nope")})
	if err == nil {
		t.Fatal("expected error for no matches")
	}
}

func TestApplySkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "a.conf"), []byte("${DATABASE}"), 0o644)

	cfg := Config{}
	if _, err := apply(cfg, []string{filepath.Join(dir, "*")}); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

// ── createBackup ─────────────────────────────────────────────────────────────

func TestCreateBackup(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "original.txt")

	os.WriteFile(file, []byte("hello"), 0o755)

	if err := createBackup(file); err != nil {
		t.Fatalf("createBackup: %v", err)
	}

	got, _ := os.ReadFile(file + ".bak")
	if string(got) != "hello" {
		t.Errorf("backup content: got %q", string(got))
	}

	info, _ := os.Stat(file + ".bak")
	if info.Mode().Perm() != 0o755 {
		t.Errorf("backup permissions: got %v", info.Mode().Perm())
	}
}

// ── splitOnDoubleDash ────────────────────────────────────────────────────────

func TestSplitOnDoubleDash(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantBefore []string
		wantAfter  []string
	}{
		{"no dash", []string{"-v", "*.conf"}, []string{"-v", "*.conf"}, nil},
		{"with dash", []string{"-v", "*.conf", "--", "/bin/sh"}, []string{"-v", "*.conf"}, []string{"/bin/sh"}},
		{"dash only", []string{"--"}, nil, nil},
		{"empty", nil, nil, nil},
		{"exec with args", []string{"*.conf", "--", "/bin/sh", "-c", "echo hi"}, []string{"*.conf"}, []string{"/bin/sh", "-c", "echo hi"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before, after := splitOnDoubleDash(tt.args)
			if !sliceEqual(before, tt.wantBefore) {
				t.Errorf("before: got %v, want %v", before, tt.wantBefore)
			}
			if !sliceEqual(after, tt.wantAfter) {
				t.Errorf("after: got %v, want %v", after, tt.wantAfter)
			}
		})
	}
}

// ── loadEnv ──────────────────────────────────────────────────────────────────

func TestLoadEnv(t *testing.T) {
	env := loadEnv()
	if v, ok := env["DATABASE"]; !ok || v != "db.example.com" {
		t.Errorf("DATABASE: got %q, ok=%v", v, ok)
	}
	if v, ok := env["NULL"]; !ok || v != "" {
		t.Errorf("NULL: got %q, ok=%v", v, ok)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func sliceEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
