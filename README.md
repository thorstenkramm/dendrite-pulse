# Dendrite

## At a glance

Dendrite is a web-based file manager, command executor and terminal application to manage a host entirely via a browser.
The project is split into two parts:

1. dendrite-pulse: A RESTful API that gives access to files, commands and the command line shell of a host.
2. dendrite-echo: A responsive single-page application providing a user-friendly graphical user interface for dendrite-pulse.

## Tech stack

dendrite-pulse, the backend, is implemented in Golang. The tech stack is described in `./rules/techstack.md`

## Usage

<!-- End-user documentation goes here -->

## Development

Development requires the following tools to be installed on your machine:

- `go`, version 1.25 or newer
- `golangci-lint`, version 2.5.0 or newer
- `node`, version 20, or newer

### Test locally with native tools

To run all linters and tests, locally proceed as follows:

```bash
# Go lint
golangci-lint run

# Go tests
go test -race -v ./...

# Markdown lint
npx markdownlint-cli "**/*.md"

# Code duplication check
npx jscpd --pattern "**/*.go" --ignore "**/*_test.go" --threshold 0 --exitCode 1

# API doc lint
npx @redocly/cli lint --lint-config off ./api-doc/openapi.yaml
```

### Test with act

[act](https://github.com/nektos/act) is a CLI tool that runs GitHub Actions locally by emulating the GitHub runner
inside Docker.

```bash
which act || brew install act
act --container-architecture linux/amd64 push -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-latest
```

### Configuration

Configuration uses TOML with `main` and `log` sections. The default file path is `/etc/dendrite/dendrite.conf`
and can be changed via `--config`.

Defaults (listen `127.0.0.1`, port `3000`, log-level `info`, log-format `text`, logging off) are applied first, then
values are overridden in this order:

- Config file (`main.listen`, `main.port`, `log.level`, `log.format`, `log.file`)
- Environment variables (`DENDRITE_MAIN_LISTEN`, `DENDRITE_MAIN_PORT`, `DENDRITE_LOG_FILE`, `DENDRITE_LOG_LEVEL`,
  `DENDRITE_LOG_FORMAT`)
- Command-line flags (`--listen`, `--port`, `--log-file`, `--log-level`, `--log-format`)

Example config:

```toml
[main]
listen = "0.0.0.0"
port = 3000

[log]
level = "info"
format = "json"
file = "/var/log/dendrite.log"
```

Validate configuration without starting the server:

```bash
./dendrite run --config-check
```
