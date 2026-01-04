# Dendrite

## At a glance

Dendrite is a web-based file manager, command executor and terminal application to manage a host entirely via a browser.
The project is split into two parts:

1. dendrite-pulse: A RESTful API that gives access to files, commands and the command line shell of a host.
2. dendrite-echo: A responsive single-page application providing a user-friendly graphical user interface for
   dendrite-pulse.

## Tech stack

dendrite-pulse, the backend, is implemented in Golang. The tech stack is described in `./rules/techstack.md`

## Usage

Dendrite-pulse provides binary releases for Linux and macOS on the
[GitHub releases page](https://github.com/thorstenkramm/dendrite-pulse/releases).

Download the archive matching your platform and architecture (the suffix matches `uname`/`uname -m`), extract it,
and install the binary alongside the example configuration.

```bash
tar -xzf dendrite-pulse_Darwin_arm64.tar.gz
sudo install -m 0755 dendrite-pulse /usr/local/bin/dendrite-pulse
sudo install -m 0644 dendrite-pulse.conf.example /etc/dendrite/dendrite.conf
```

## Development

Development requires the following tools to be installed on your machine:

- `go`, version 1.25 or newer
- `golangci-lint`, version 2.5.0 or newer
- `node`, version 20, or newer
- `goreleaser`, version 2.13.2

### Test locally with native tools

To run all linters and tests, locally proceed as follows:

```bash
# Go lint
golangci-lint run

# Go lint with Linux (worth doing on macOS)
docker run --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.5.0 golangci-lint run ./...

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

### Build release archives locally

Create a local snapshot build without publishing a GitHub release:

```bash
goreleaser release --snapshot --clean
```

The generated archives are written to `./dist`.

### Configuration

Configuration uses TOML with `main` and `log` sections. The default file path is `/etc/dendrite/dendrite.conf`
and can be changed via `--config`.

File roots are required and map virtual folders to real directories. Use `--file-root /virtual:/source` (repeatable or
comma-separated) or `DENDRITE_FILE_ROOT` with the same syntax. In TOML, use `[[file-root]]` tables.

Defaults (listen `127.0.0.1`, port `3000`, log-level `info`, log-format `text`, logging off) are applied first, then
values are overridden in this order:

Defaults also include `max_upload_size` ("2GB") and `file_mode` ("0600").

- Config file (`main.listen`, `main.port`, `main.max_upload_size`, `main.file_mode`, `log.level`, `log.format`,
  `log.file`, `file-root`)
- Environment variables (`DENDRITE_MAIN_LISTEN`, `DENDRITE_MAIN_PORT`, `DENDRITE_MAIN_MAX_UPLOAD_SIZE`,
  `DENDRITE_MAIN_FILE_MODE`, `DENDRITE_LOG_FILE`, `DENDRITE_LOG_LEVEL`, `DENDRITE_LOG_FORMAT`,
  `DENDRITE_FILE_ROOT`)
- Command-line flags (`--listen`, `--port`, `--max-upload-size`, `--file-mode`, `--log-file`, `--log-level`,
  `--log-format`, `--file-root`)

Example config:

```toml
[main]
listen = "0.0.0.0"
port = 3000
max_upload_size = "2GB"
file_mode = "0600"

[log]
level = "info"
format = "json"
file = "/var/log/dendrite.log"

[[file-root]]
virtual = "/public"
source = "/var/www/public"
```

Validate configuration without starting the server:

```bash
./dendrite run --config-check
```
