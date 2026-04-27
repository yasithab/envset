# Envset

Set environment variables into configuration files.

---

## How it works

Envset parses arbitrary configuration files (using glob patterns) and replaces all references to environment variables with their values - inline (the files are modified in place).

**Supported syntax:**

- `${KEY}` - replaced if `KEY` is set in the environment, left as-is otherwise
- `${KEY:-default value}` - uses the default if `KEY` is not set

Unset variables without defaults are silently skipped - safe for files that use `${...}` for other purposes (Node.js templates, shell scripts, etc.). Use `--strict` to error on any unset variable.

---

## Install

```bash
# Build from source
go build -o envset .

# Install globally
sudo mv envset /usr/local/bin/envset

# macOS: remove quarantine if needed
xattr -d com.apple.quarantine /usr/local/bin/envset
chmod +x /usr/local/bin/envset
```

---

## Quick start

```bash
# Replace variables in a config file
envset /etc/app.conf

# Dry-run - see output without modifying files
envset -d /etc/app.conf

# Verbose mode with backup
envset -v -b /etc/nginx/*.conf

# Process configs then exec another command
envset -v /etc/app.conf -- /usr/bin/myapp --config /etc/app.conf
```

---

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--backup` | `-b` | `false` | Create `.bak` backup before modifying |
| `--dry-run` | `-d` | `false` | Output to stdout instead of inline replacement |
| `--strict` | `-s` | `false` | Fail on any unset variable (with or without defaults) |
| `--verbose` | `-v` | `false` | Verbose logging with summary stats |
| `--prefix` | `-p` | `""` | Only expand variables matching this prefix |
| `--env-file` | `-e` | `""` | Load additional env vars from a file |
| `--version` | | | Print version and exit |

---

## Prefix mode

Use `--prefix` to only expand variables matching a specific prefix. All other `${...}` patterns are left untouched regardless of whether the variable is set.

```bash
# Only expand variables starting with EP_
envset -p EP_ /etc/app.conf
```

Given `EP_HOST=api.example.com` in the environment:

```
# Before
server_name=${EP_HOST}
template_var=${name}

# After
server_name=api.example.com
template_var=${name}
```

This is the safest option for files that mix envset variables with other `${...}` syntax.

---

## Env file

Load additional variables from a `.env` file before processing. Values from the env file override system environment variables.

```bash
envset -e .env /etc/app.conf
```

Supported `.env` format:

```bash
# Comments are ignored
DB_HOST=localhost
DB_PORT=5432
DB_NAME="mydb"
DB_PASS='s3cret'
export API_KEY=abc123
```

---

## Exec mode

Pass `--` after all arguments to exec another command after template processing:

```bash
envset -v /etc/nginx/*.conf -- /usr/sbin/nginx -g "daemon off;"
```

This replaces the current process with the specified command - useful as a Docker `CMD` entrypoint.

---

## Escaping

If the file already contains `${...}` patterns that should not be replaced, escape them with a leading backslash:

- `\${VAR}` → `${VAR}` (escaped, not replaced)
- `\\${VAR}` → `\<value>` (not escaped, replaced)

Even number of leading backslashes = replaced. Odd number = escaped.

---

## Full example

```
$ cat /etc/foo.conf
Database=${FOO_DATABASE}
DatabaseSlave=${BAR_DATABASE:-db2.example.com}
Mode=fancy
Escaped1=\${FOO_DATABASE}
NotEscaped1=\\${FOO_DATABASE}

$ export FOO_DATABASE=db.example.com

$ envset -v /etc/f*.conf
Processing /etc/foo.conf
Expanding "FOO_DATABASE" in /etc/foo.conf
Using default "db2.example.com" for "BAR_DATABASE" in /etc/foo.conf
Expanding "FOO_DATABASE" in /etc/foo.conf
Done: 1 file(s), 2 expanded, 1 defaults, 0 skipped

$ cat /etc/foo.conf
Database=db.example.com
DatabaseSlave=db2.example.com
Mode=fancy
Escaped1=${FOO_DATABASE}
NotEscaped1=\db.example.com
```

---

## Docker usage

```dockerfile
FROM nginx:latest

COPY envset /usr/local/bin/envset
RUN chmod +x /usr/local/bin/envset

CMD ["/usr/local/bin/envset", "-v", "/etc/nginx/*.conf", "--", "/usr/sbin/nginx", "-g", "daemon off;"]
```

With an env file:

```dockerfile
CMD ["/usr/local/bin/envset", "-e", "/run/secrets/.env", "-v", "/etc/nginx/*.conf", "--", "/usr/sbin/nginx", "-g", "daemon off;"]
```
