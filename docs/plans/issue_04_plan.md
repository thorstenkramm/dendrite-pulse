# Plan: Issue #4 — API GET Files

Planning for implementing multi-root file listing and download under `/api/v1/files`.

## Step 1: Configuration model and validation

Status: completed

- Add `file_root` slice to config structs with `virtual` and `source` fields matching TOML `[[file-root]]`.
- Extend loader to parse `--file-root` flag and `DENDRITE_FILE_ROOT` env (comma-separated `virtual:source` pairs).
- Implement validation rules: both start with slash, single-folder virtual, no trailing spaces, sources exist.
- Ensure resolved roots are normalized and deduplicated by virtual name.

Testing:

- Table-driven config tests covering defaults, TOML array, env/flag precedence, validation failures, and duplicate virtuals.

## Step 2: CLI wiring and startup guards

Status: completed

- Add Cobra flag `--file-root` (repeatable and comma-separated support) binding to Viper.
- Enforce presence of at least one root at startup; return clear error on invalid roots.
- Include root info in `--config-check` output/log for visibility.

Testing:

- Cobra/Viper integration tests for flag/env parsing and `config-check` path with missing/invalid roots.

## Step 3: Path resolution and security

Status: completed

- Implement resolver that maps request path to root and real path; reject traversal and escaped paths.
- Resolve symlinks and ensure final path stays under its source root; handle cycles safely.
- Normalize virtual paths (trailing slash, repeated slashes) for consistent behavior.

Testing:

- Unit tests with fixtures containing traversal attempts, symlinks inside/outside root, and odd characters.

## Step 4: Filesystem metadata service

Status: in progress

- Build service to stat entries and classify `file`, `folder`, `symlink`; follow symlink target for type/size.
- Collect metadata: size, permission mode (octal string), uid/gid and names, mime type (based on content sniffing),
  accessed/modified/changed/born times (null when unavailable).
- Support directory listing with virtual names; include root-level listing returning available virtual roots.

Testing:

- Service-level tests over temp directories with files, dirs, symlinks, and unusual names; verify metadata fields.

## Step 5: HTTP handlers and routing

Status: pending

- Add Echo routes for `GET /api/v1/files` (roots), `GET /api/v1/files/*` (list or download).
- For directories: return JSON:API-compliant list of resource objects with metadata.
- For files/symlinks: stream content with correct mime type; support range if feasible via Echo helpers.
- Map filesystem errors to HTTP codes (404 missing, 403 permission, 400 invalid path, 500 fallback).

Testing:

- Handler tests with httptest server covering directory listing, file download, symlink resolution, and error mapping.
- JSON structure assertions per `rules/json-api.md`.

## Step 6: Documentation and OpenAPI

Status: pending

- Document config options (file roots) and example usage in README and sample config.
- Update `api-doc/openapi.yaml` and endpoint file for `/api/v1/files` with parameters, responses, and examples.
- Run `npx @redocly/cli lint --lint-config off ./api-doc/openapi.yaml`.

Testing:

- Markdown lint on docs; redocly lint clean; manual check that examples align with behavior.

## Step 7: Final validation

Status: pending

- Run `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go test -race -v ./...`,
  `npx jscpd --pattern "**/*.go" --ignore "**/*_test.go" --threshold 0 --exitCode 1`.
- Smoke test by starting server with sample roots and exercising list/download via curl.
