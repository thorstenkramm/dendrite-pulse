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
