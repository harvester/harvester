---
applyTo: "**/*.go, go.mod, go.sum"
---

# Golang Security Review Instructions

When reviewing changes to Go code, perform the following security checks:

## Evaluate Active gosec Rules

Check changed code against every gosec rule that is **not** suppressed in `.golangci.yaml`. Pay particular attention to:

| Rule | What to look for |
|------|-----------------|
| G102 | Binding to all network interfaces (`0.0.0.0`) without justification |
| G103 | Use of `unsafe` package |
| G104 | Errors unhandled (complements `errcheck`) |
| G107 | URL built from variable input passed to HTTP request — flag if not validated |
| G201 / G202 | SQL query string built with `fmt.Sprintf` or string concatenation |
| G204 | `exec.Command` called with variable arguments — verify args are not user-controlled |
| G304 | File path opened from variable — check for path traversal |
| G305 | File path from `Zip`/`Tar` entry — flag if extraction destination is not sanitised |
| G306 | File created with permissions above `0644` (threshold set in `.golangci.yaml`) |
| G307 | `defer` on a function that returns an error without the error being checked |
| G404 | `math/rand` used instead of `crypto/rand` for security-sensitive values |
| G501–G504 | Imports of `crypto/md5`, `crypto/sha1`, `crypto/des`, `crypto/rc4` |
| G601 | Implicit memory aliasing in `for` loop (pre-Go 1.22 semantics) |

Exception: Report violations of rule G101 (hardcoded credentials) even though it's suppressed in `.golangci.yaml`.

For each finding, cite the file, line, rule ID, and a short explanation of the risk.

The full list of gosec rules can be found [here](https://raw.githubusercontent.com/securego/gosec/refs/heads/master/RULES.md).

## Report Deprecated Dependencies

If changes are made to `go.mod` or `go.sum`, check for any dependencies that are marked as deprecated in their module repository (e.g., GitHub). Note that deprecation is a strong signal of unmaintained code and potential security risks, especially if the deprecation notice mentions security issues. These should be reported with a 'High' severity rating.
