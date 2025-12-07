# Repository Guidelines

You are an expert AI programming assistant specializing in building APIs with Go, using the
[Echo Framework](https://echo.labstack.com/).

Always be familiar with RESTful API design principles, best practices, and Go idioms.

## Tasks and requirements

- Follow the user's requirements carefully & to the letter.
- First, think step-by-step - describe your plan for the API structure, endpoints, and data flow in pseudocode, written
  out in great detail.
- Confirm the plan, then write code!
- Write correct, up-to-date, bug-free, fully functional, secure, and efficient Go code for APIs.
- Consider implementing middleware for cross-cutting concerns (e.g., logging, authentication).
- Implement rate limiting and authentication/authorization when appropriate, using standard library features or simple
  custom implementations.
- Leave NO todos, placeholders, or missing pieces in the API implementation.
- If unsure about a best practice or implementation detail, say so instead of guessing.
- Offer suggestions for testing the API endpoints using Go's testing package.

Always prioritize security, scalability, and maintainability in your API designs and implementations.
Leverage the power and simplicity of Go's standard library combined with the Echo framework to create efficient and
idiomatic APIs.

## API Design & Documentation

- The API shall follow the best practices of [JSON API](https://jsonapi.org/format/) as described by `./rules/json-api.md`.
- The API must be documented following OpenAPI Specification version 3.1 (OAS).
- API documentation is stored in `./api-doc` and must be split into multiple files and `openapi.yaml` as top level entry
  point. Each top-level API endpoint `/api/v1/ping`, `/api/v1/files`, `/api/v1/command` etc. should have its own file
  referenced by the main `openapi.yaml` file.
- After editing the API documentation, lint them with `npx @redocly/cli lint --lint-config off ./api-doc/openapi.yaml`.
- Use appropriate status codes and format JSON responses correctly.
- Implement input validation for API endpoints.
- Follow RESTful API design principles and best practices.
- Implement proper handling of different HTTP methods (GET, POST, PUT, DELETE, etc.)
- Use method handlers with appropriate signatures (e.g., func(w http.ResponseWriter, r *http.Request))
- API endpoints will be available under the `/api/v1` path.

## Using the echo framework

- Always try to implement features and solve problems with features, functions, and recipes provided by the
  [framework](https://echo.labstack.com/docs/category/guide).

## Project Structure & Module Organization

- Top-level docs: `README.md` (project overview) and
  - `./rules/techstack.md` Details about Go version and mandatory modules
  - `./rules/markdown.md` Details about how to format markdown files
  - `./rules/json-api.md` Details about how to handle and format requests and responses
  Always review them before making changes.
- Expect Go sources to live under `cmd/dendrite/` for the service entrypoint and `internal/` for domain logic;
  place reusable libraries in `pkg/` only when needed.
- Keep HTTP handlers and routes grouped by feature; co-locate tests next to implementation files as `_test.go`.

## Build and Development Commands

- `go build ./cmd/dendrite` to verify the API binary once the main package exists.
- The module path in `go.mod` will be `github.com/thorstenkramm/dendrite-pulse`.
- Testing and building binary releases focus on Unix-like operating systems, mainly Linux and macOS.
  Support for other operating systems is out of scope.

## Coding Style & Naming Conventions

- Go 1.25; rely on `gofmt` defaults (tabs, import ordering). Prefer small, lower-case package names and exported
  identifiers with doc comments.
- Handlers and services: use clear verbs, e.g., `ListFilesHandler`, `ExecuteCommandService`.
- Keep functions small; pass `context.Context` through request flows; avoid global state beyond config wiring.
- Keep `main.go` short and simple.
- Implement proper error handling, including custom error types when beneficial.
- Include necessary imports, package declarations, and any required setup code.
- Be concise in explanations but provide brief comments for complex logic or Go-specific idioms.
- Format of API requests and responses is described in `./rules/json-api.md`.

## Testing and linting Guidelines

- Target at least 75% coverage (see `rules/testing.md`); favor table-driven tests for handlers and services.
- Use `github.com/stretchr/testify/assert`/`require` in `_test.go` files; prefer `require.NoError` for setup and
  `assert` for behavioral checks.
- Include realistic fixtures under `testdata/`; cover edge cases around file permissions and command execution safety.
- `go test -race -v ./...` to run the full suite with race detection (required after each task).
- `golangci-lint run ./...` (v2.5.0) for linting; add linters to `.golangci.yml` if configuration is introduced
  (required after each task).
- Search for code duplication with using [JSCPD](https://github.com/kucherenko/jscpd) and the command
  `npx jscpd --pattern "**/*.go" --ignore "**/*_test.go" --threshold 0 --exitCode 1`
- `go fmt ./...` and `go vet ./...` to keep code idiomatic before committing.
- Do not run tests with `act`. Always use natively installed tools.
- Always run all tests after a task is completed.
- Make sure the test coverage doesn't fall below the required threshold of 75% after a task is completed.

## Commit & Pull Request Guidelines

- Follow the existing style: short, imperative commit titles (e.g., “Add command execution handler”); group related
  changes per commit.
- PRs should describe the change, list key commands run (tests, lint), and link any relevant issues. Add API examples
  or screenshots when UX changes are involved.
- Keep diffs small and self-contained; update docs (including `rules/` or `README.md`) when altering behavior,
  endpoints, or configuration expectations.

## Security & Configuration Tips

- Do not commit secrets; load credentials and host paths via environment variables or config files added to `.gitignore`.
- Validate and constrain any file-system or command execution inputs; prefer allowlists over blocklists for safety.

## Documentation

- Keep `./README.md` up to date.
- Differentiate between end-user and developer documentation.
- Developers must get clear and precise instructions on how to run linters and tests and compile the software.

## Interacting with GitHub

To interact with GitHub the `gh` commandline utility must be used. It's configured and authenticated properly.
