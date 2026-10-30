# Middleware checks

Run `Scripts/test.sh` from the repository root. It runs the Go test suites, Docker smoke checks or both. Tests use disposable settings without mounting your `.env` file or sending real mail and notifications.


## Usage

```bash
./Scripts/test.sh [unit|e2e|all] [--no-build] [--keep]
```

Choose one mode and add flags as needed.

| Argument | Default | What it does |
|---|---|---|
| `unit` | No | Runs every Go suite with `go test -race -count=1 ./...`, including unit, compatibility, HTTP, executable, runner and privacy checks. Results are not reused from the test cache. |
| `e2e` | No | Builds the Docker image, starts a test container and checks its health and HTTP responses. Optional live checks run only when their fixture variables are set. |
| `all` | Yes | Runs `unit`, then `e2e`. A Go test failure stops the run before Docker starts. Omitting the mode selects `all`. |
| `--no-build` | Off | Skips the Docker build and reuses the image selected by `E2E_IMAGE`. Fails if that image does not exist locally. Applies to `e2e` and `all`. |
| `--keep` | Off | Leaves the test container running after the smoke checks, including when a check fails. Applies to `e2e` and `all`. |
| `-h`, `--help` | Off | Prints usage and exits without running tests. |

## Common commands

| Command | Purpose |
|---|---|
| `./Scripts/test.sh unit` | Run all Go tests without Docker or live provider access. |
| `./Scripts/test.sh` | Run all Go tests, then build and test the Docker image. |
| `E2E_SKIP_ONLINE=1 ./Scripts/test.sh e2e` | Build the image and run container checks without the deleted-link checks. Leave both live-path variables unset for offline smoke checks. |
| `E2E_SKIP_ONLINE=1 ./Scripts/test.sh e2e --no-build` | Test the existing local `middleware:test` image. |
| `./Scripts/test.sh e2e --keep` | Keep the container available for inspection after testing. |
| `GO_BIN=/opt/homebrew/bin/go ./Scripts/test.sh unit` | Select an existing Go compiler explicitly. |
| `E2E_IMAGE=middleware:review E2E_PORT=8100 ./Scripts/test.sh e2e` | Build and test a custom image tag on host port `8100`. |

## Environment variables

Set variables before the command, as shown above. These are test-runner settings and do not need to be added to `.env`.

| Variable | Default | Accepted value and behavior |
|---|---|---|
| `GO_BIN` | Auto-discovered | Executable path or command for Go. Used by `unit` and `all`, including child builds in executable tests. Quote paths containing spaces. |
| `E2E_IMAGE` | `middleware:test` | Local Docker image tag to build or reuse with `--no-build`. |
| `E2E_PORT` | `8099` | Available host TCP port mapped to container port `8080`. Requests use `http://127.0.0.1:<port>`. |
| `E2E_SKIP_ONLINE` | Unset | Any nonempty value, including `0`, skips the deleted/invalid Dropbox link group. Use `1` to enable skipping. It does not disable the two live-path checks below. |
| `E2E_DELETED_ID` | Unset | Share ID from an explicitly selected disposable deleted Dropbox link. Requires `E2E_DELETED_RLKEY` from the same link. |
| `E2E_DELETED_RLKEY` | Unset | The `rlkey` value for that deleted link. The deleted/invalid group is skipped if either deleted-link variable is missing or Dropbox is unreachable. |
| `E2E_INSTALL_PATH` | Unset | Request path beginning with `/install/` for a known live public manifest. Enables checks for the manifest status, XML, IPA download URL and content type. |
| `E2E_APPINFO_PATH` | Unset | Request path beginning with `/appinfo/` for known live public metadata. Enables checks for status and valid JSON. Requires Python 3. |

Live paths are relative to the local test server, rather than complete Dropbox URLs. For example, replace the placeholders below with an explicitly selected public test build. Keep real link identifiers out of committed files.

```bash
E2E_INSTALL_PATH='/install/scl/fi/<id>/queryparam-rlkey-value-<key>/manifest.plist' \
E2E_APPINFO_PATH='/appinfo/scl/fi/<id>/appinfo.json?rlkey=<key>' \
./Scripts/test.sh e2e
```

For offline smoke checks, unset all live fixture variables and reuse an already built image:

```bash
unset E2E_DELETED_ID E2E_DELETED_RLKEY E2E_INSTALL_PATH E2E_APPINFO_PATH
E2E_SKIP_ONLINE=1 ./Scripts/test.sh e2e --no-build
```

## Results and cleanup

The summary reports passed, failed and skipped checks. A failed check or setup error exits with a nonzero status. Optional live checks report a skip when their fixtures are absent.

Each Docker run replaces any existing container named `middleware-e2e`. The script removes the container and temporary response files when it exits, unless `--keep` retains the container. Inspect and remove a retained container with:

```bash
docker logs middleware-e2e
docker rm -f middleware-e2e
```
