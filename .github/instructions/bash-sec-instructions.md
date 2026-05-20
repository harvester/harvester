---
applyTo: "package/**/*.sh, scripts/**/*.sh, **/*.sh"
---

# Bash Security Review Instructions

When reviewing changes to shell scripts in the `scripts/` or `package/` folders, perform the following security checks:

## Check Shell Script Vulnerabilities

Check for common shell script vulnerabilities and bad practices, especially in any new or modified scripts. Pay particular attention to:

| Issue | What to look for |
|-------|----------------- |
| **Unquoted variables** | Variables used without quotes (e.g., `$var` instead of `"$var"`) — can cause word splitting and glob expansion |
| **Command injection** | User input or variables interpolated into commands without sanitization — especially with `eval`, backticks, or `$(...)` |
| **Path traversal** | File paths constructed from input without validation — check for `..` sequences and absolute path handling |
| **Unsafe `eval`** | Any use of `eval` with user-controlled input or variables |
| **Unchecked commands** | Commands that can fail silently — ensure `set -e` or explicit error checking with `\|\| exit 1` |
| **Insecure temp files** | Predictable temp file names instead of `mktemp` — can lead to race conditions |
| **World-writable files** | Files/directories created with overly permissive permissions (666, 777) |
| **Missing input validation** | Script arguments (`$1`, `$2`, etc.) used without validation or bounds checking |
| **Unsafe downloads** | `curl` or `wget` without integrity checks (checksum validation) or used with `\| sh` |
| **Secret exposure** | Secrets/credentials in script output, logs, or error messages |
| **Missing ShellCheck** | Scripts not validated with ShellCheck — recommend running `shellcheck script.sh` |

Common safe patterns:

- Always quote variables: `"$VAR"` not `$VAR`
- Use `set -euo pipefail` at script start for safer error handling
- Validate arguments: `[[ -z "${1:-}" ]] && { echo "Error: missing argument"; exit 1; }`
- Use `mktemp` for temporary files: `temp_file=$(mktemp)`
- Check command success: `command || { echo "command failed"; exit 1; }`
- Use arrays for commands with arguments: `cmd=("binary" "arg1" "arg2"); "${cmd[@]}"`

For each shell script finding, cite the file, line number, issue type, and recommended fix.
