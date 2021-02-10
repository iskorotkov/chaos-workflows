# Chaos Workflows

Service providing REST API for working with Argo workflows.

- [Chaos Workflows](#chaos-workflows)
  - [Env vars](#env-vars)
  - [REST API](#rest-api)
  - [Development](#development)

## Env vars

Service requires several env vars set (example values are provided in parentheses):

- `ARGO_SERVER` — Argo server to use (`argo-server.argo.svc:2746`)
- `DEVELOPMENT` — whether in development or not (`false`)

## REST API

- /api/v1/workflows
  - /{namespace}/{name} — upgrades connection to WebSocket connection and starts sending workflow events until the workflow is completed.

## Development

To build project:

```shell
go build ./...
```

To run tests:

```shell
go test ./...
```
