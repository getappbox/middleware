# Integration tests

This directory holds broader tests that exercise the public HTTP API and compiled executable. Production code remains in `cmd/` and `internal/`.

| Location | Purpose |
|---|---|
| `tests/http/` | Public HTTP contracts with fake upstreams, rate budgets, authentication ordering, HEAD behavior, request-size boundaries, credentials and JSON fuzzing |
| `tests/process/` | Compiled race-enabled executable, native bind, environment precedence, Unix sockets, health failures, occupied addresses and graceful SIGTERM |
| `internal/middleware/*_test.go` | Private unit tests, parser limits, fixtures, provider payloads, caching and cancellation |
| `cmd/middleware/main_test.go` | Private argument/configuration/logging unit tests |

Run everything with `go test -race -count=1 ./...`. Run just the separate suites with `go test -race ./tests/...`.
