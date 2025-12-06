# API Documentation

This folder contains the sources of rportd API documentation following the openapi 3.0.1 standard.

If you came by here to read the API documentation, go to [apidoc.example.com](https://apidoc.example.com) to switch to
the rendered HTML version.

## Build the documentation from the sources

There are many tools out there to convert the YAML sources into different formats. For example,
[Swagger Codegen](https://swagger.io/docs/open-source-tools/swagger-codegen/) or the [Open API Codegenerator](https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/5.0.0/).
Both are java command line tools.

More comfort for reading and writing Open API docs is provided by [Redoc](https://github.com/Redocly/redoc) and the
command line tool [Redoc CLI](https://redocly.com/docs/redoc/deployment/cli/).
With NodeJS installed you can directly launch the tools with `npx`. See below.

### Run a local webserver

Running a local webserver is very handy for writing the documentation. Changes to the files are immediately rendered.

```shell
npx @redocly/cli preview-docs ./api-doc/openapi.yaml
```

### Use the linter

Before pushing changes to the repository, verify the linter does not throw errors.

```shell
npx @redocly/cli lint --lint-config off ./api-doc/openapi.yaml
```

See the details about the [applied rules and their output](https://redocly.com/docs/cli/resources/built-in-rules/).

The linter is integrated into the CI/CD workflows, and merge requests are rejected if the linter throws errors or warnings.

### Render to HTML

To render the API documentation into a single dependency-free HTML file, use:

```shell
npx @redocly/cli build-docs -o index.html ./api-doc/openapi.yaml
```
