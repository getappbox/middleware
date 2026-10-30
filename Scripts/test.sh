#!/bin/bash

set -uo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"


IMAGE="${E2E_IMAGE:-middleware:test}"
CONTAINER="middleware-e2e"
PORT="${E2E_PORT:-8099}"
BASE="http://127.0.0.1:$PORT"
CLIENT_TOKEN="e2e-client-token"
APP_KEY="e2e-dropbox-app-key"

DELETED_ID="${E2E_DELETED_ID:-}"
DELETED_RLKEY="${E2E_DELETED_RLKEY:-}"
BOGUS_ID="deadbeef0000000000000"

E2E_INSTALL_PATH="${E2E_INSTALL_PATH:-}"
E2E_APPINFO_PATH="${E2E_APPINFO_PATH:-}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PASS_COUNT=0
FAIL_COUNT=0
SKIP_COUNT=0

log() { echo -e "$@"; }

section() {
    log ""
    log "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    log "${CYAN} $1${NC}"
    log "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

pass() { log "${GREEN}  ✅ PASS${NC}  $1"; PASS_COUNT=$((PASS_COUNT + 1)); }
fail() { log "${RED}  ❌ FAIL${NC}  $1"; log "          $2"; FAIL_COUNT=$((FAIL_COUNT + 1)); }
skip() { log "${YELLOW}  ⏭️  SKIP${NC}  $1 — $2"; SKIP_COUNT=$((SKIP_COUNT + 1)); }
die()  { log "${RED}$1${NC}"; exit 1; }


HDR=""
BODY=""

request() {
    RES_STATUS=$(curl -sS --path-as-is -o "$BODY" -D "$HDR" -w '%{http_code}' \
                      --max-time 45 "$BASE$1" 2>/dev/null) || RES_STATUS="000"
    RES_PATH="$1"
    RES_HEADER_BYTES=$(wc -c < "$HDR" | tr -d ' ')
    RES_BODY_BYTES=$(wc -c < "$BODY" | tr -d ' ')
}

header_value() { awk -v k="$(echo "$1" | tr 'A-Z' 'a-z'):" 'tolower($1)==k {sub(/^[^:]*: */,""); gsub(/\r/,""); print}' "$HDR"; }

assert_status() {  # <name> <expected status>
    if [ "$RES_STATUS" = "$2" ]; then
        pass "$1 ($2)"
    else
        fail "$1" "expected $2, got $RES_STATUS for the selected request"
    fi
}

assert_body_contains() {  # <name> <substring>
    if grep -qF -- "$2" "$BODY"; then
        pass "$1"
    else
        fail "$1" "body of the selected request does not contain '$2'"
    fi
}

assert_header_absent() {  # <name> <header>
    if [ -z "$(header_value "$2")" ]; then
        pass "$1"
    else
        fail "$1" "$2 was forwarded from upstream"
    fi
}

assert_header_value() {  # <name> <header> <expected>
    local actual
    actual="$(header_value "$2")"
    if [ "$actual" = "$3" ]; then
        pass "$1"
    else
        fail "$1" "$2 is '$actual', expected '$3' for the selected request"
    fi
}

assert_header_budget() {  # <name> <max bytes>
    if [ "$RES_HEADER_BYTES" -lt "$2" ]; then
        pass "$1 (${RES_HEADER_BYTES} bytes)"
    else
        fail "$1" "response headers are ${RES_HEADER_BYTES} bytes, budget is $2 for the selected request"
    fi
}

assert_content_length_matches_body() {  # <name>
    local declared
    declared="$(header_value content-length)"
    if [ "$declared" = "$RES_BODY_BYTES" ]; then
        pass "$1 (${RES_BODY_BYTES} bytes)"
    else
        fail "$1" "Content-Length says '${declared}', body is ${RES_BODY_BYTES} bytes for the selected request"
    fi
}

check_status()        { request "$2"; assert_status "$1" "$3"; }
check_body_contains() { request "$2"; assert_body_contains "$1" "$3"; }


run_unit_tests() {
    section "Go test suites"
    resolve_go
    if (cd "$REPO_DIR" && "$GO_BIN" test -race -count=1 ./...); then
        pass "go test -race -count=1 ./..."
    else
        fail "go test -race -count=1 ./..." "see output above"
        return 1
    fi
}

resolve_go() {
    local candidate="" resolved=""
    if [ -n "${GO_BIN:-}" ]; then
        candidate="$GO_BIN"
    elif type -P go >/dev/null 2>&1; then
        candidate="$(type -P go)"
    else
        for resolved in /opt/homebrew/bin/go /usr/local/bin/go /usr/local/go/bin/go; do
            if [ -x "$resolved" ] && [ ! -d "$resolved" ]; then
                candidate="$resolved"
                break
            fi
        done
    fi

    if [ -z "$candidate" ]; then
        die "Go is required for unit and integration tests. Install Go 1.27.3 or later:
  macOS with Homebrew: brew install go
  Other installations: https://go.dev/doc/install
Then rerun ./Scripts/test.sh unit, or select an existing compiler:
  GO_BIN=/absolute/path/to/go ./Scripts/test.sh unit"
    fi

    resolved="$(type -P "$candidate")" || die "GO_BIN does not identify an executable: $candidate"
    [ -x "$resolved" ] && [ ! -d "$resolved" ] || die "GO_BIN does not identify an executable: $candidate"
    GO_BIN="$(cd "$(dirname "$resolved")" && pwd)/$(basename "$resolved")"
    export GO_BIN
    "$GO_BIN" version || die "Go could not start. Check GO_BIN or reinstall Go."
}


KEEP=0
NO_BUILD=0
CONTAINER_STARTED=0

cleanup() {
    rm -f "$HDR" "$BODY"
    if [ "$KEEP" = "1" ] && [ "$CONTAINER_STARTED" = "1" ]; then
        log ""
        log "${YELLOW}Container left running: $CONTAINER on $BASE (docker rm -f $CONTAINER to stop)${NC}"
    elif [ "$CONTAINER_STARTED" = "1" ]; then
        docker rm -f "$CONTAINER" >/dev/null 2>&1
    fi
}

start_container() {
    docker rm -f "$CONTAINER" >/dev/null 2>&1

    if [ "$NO_BUILD" = "0" ]; then
        log "Building $IMAGE (building the Go server)…"
        docker build -t "$IMAGE" "$REPO_DIR" >/dev/null || die "docker build failed"
    elif ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
        die "--no-build was passed but $IMAGE does not exist yet"
    fi

    docker run -d --name "$CONTAINER" -p "$PORT:8080" \
        -e APPBOX_CLIENT_TOKEN="$CLIENT_TOKEN" \
        -e DROPBOX_APP_KEY="$APP_KEY" \
        -e MAILGUN_API_KEY="e2e-mailgun-key" \
        -e YOURLS_SIGNATURE_SECRET="e2e-yourls-secret" \
        -e LOG_LEVEL="info" \
        "$IMAGE" >/dev/null || die "docker run failed"
    CONTAINER_STARTED=1

    local waited=0
    until curl -sf --max-time 2 "$BASE/" >/dev/null 2>&1; do
        waited=$((waited + 1))
        [ "$waited" -ge 45 ] && { docker logs "$CONTAINER"; die "container never became ready on $BASE"; }
        sleep 1
    done
    log "Container up on $BASE"
}

run_e2e_tests() {
    command -v docker >/dev/null 2>&1 || die "Docker is not installed"
    docker info >/dev/null 2>&1 || die "Docker is installed but the daemon is not running"

    trap cleanup EXIT
    HDR="$(mktemp)" || die "Could not create response header file"
    BODY="$(mktemp)" || die "Could not create response body file"
    section "End-to-end — Docker image"
    start_container
    if docker exec "$CONTAINER" /app/middleware --healthcheck --env production; then
        pass "container health command succeeds"
    else
        fail "container health command succeeds" "health command exited with an error"
    fi

    log ""
    log "Service and request validation"
    check_body_contains "health check serves its banner" "/" "AppBox Install Service Helper"
    assert_content_length_matches_body "health response length matches body"
    check_status "install rejects a non-share path"       "/install/notascl/manifest.plist" "400"
    check_status "install rejects path traversal"         "/install/scl/../../etc/passwd" "400"
    check_status "appinfo rejects a non-share path"       "/appinfo/scl/../../etc/passwd" "400"
    check_status "cors requires a url"                    "/cors" "400"
    check_status "cors refuses a host off the allowlist"  "/cors?url=https://evil.example.com/x" "403"
    check_status "cors refuses plain http"                "/cors?url=http://www.dropbox.com/scl/x" "403"
    check_status "api/v1 refuses a missing client token"  "/api/v1/config" "401"

    request "/api/v1/config"
    if curl -sS --max-time 10 -H "X-AppBox-Client-Token: $CLIENT_TOKEN" "$BASE/api/v1/config" | grep -qF "$APP_KEY"; then
        pass "api/v1 serves config to a valid client token"
    else
        fail "api/v1 serves config to a valid client token" "expected the app key in the response"
    fi

    log ""
    log "Dropbox notice pages (needs network)"
    if [ -n "${E2E_SKIP_ONLINE:-}" ]; then
        skip "every Dropbox-backed check" "E2E_SKIP_ONLINE is set"
    elif [ -z "$DELETED_ID" ] || [ -z "$DELETED_RLKEY" ]; then
        skip "every Dropbox-backed check" "Set E2E_DELETED_ID and E2E_DELETED_RLKEY for an explicitly selected deleted-link fixture"
    elif ! curl -sf -o /dev/null --max-time 15 "https://www.dropbox.com/robots.txt"; then
        skip "every Dropbox-backed check" "www.dropbox.com is unreachable from here"
    else
        local deleted_install="/install/scl/fi/$DELETED_ID/queryparam-rlkey-value-$DELETED_RLKEY/manifest.plist"
        local deleted_appinfo="/appinfo/scl/fi/$DELETED_ID/manifest.plist?rlkey=$DELETED_RLKEY"
        local bogus_install="/install/scl/fi/$BOGUS_ID/queryparam-rlkey-value-zzzz/manifest.plist"
        local deleted_cors="/cors?url=https%3A%2F%2Fwww.dropbox.com%2Fscl%2Ffi%2F$DELETED_ID%2Fmanifest.plist%3Frlkey%3D$DELETED_RLKEY%26dl%3D1"

        request "$deleted_install"
        assert_status "deleted manifest is 404, not a 502" "404"
        assert_body_contains "deleted manifest says why" "not found on Dropbox"

        check_status "invalid link is 404"             "$bogus_install" "404"
        check_status "deleted appinfo is 404"          "$deleted_appinfo" "404"
        check_status "legacy /appinfo/s still answers" "/appinfo/s/$BOGUS_ID/appinfo.json" "200"

        request "$deleted_cors"
        assert_status "proxied page keeps the upstream status" "200"
        assert_header_budget "proxied page fits a 4 KB header buffer" "1024"
        assert_header_absent "Dropbox CSP is not forwarded" "content-security-policy"
        assert_header_absent "Dropbox cookies are not forwarded" "set-cookie"
        assert_header_absent "Dropbox tracing is not forwarded" "x-dropbox-request-id"
        assert_content_length_matches_body "proxied body is intact"
    fi

    log ""
    log "Live build (set E2E_INSTALL_PATH / E2E_APPINFO_PATH to run)"
    if [ -n "$E2E_INSTALL_PATH" ]; then
        request "$E2E_INSTALL_PATH"
        assert_status "live manifest is served" "200"
        assert_body_contains "live manifest is a plist" "<key>assets</key>"
        assert_body_contains "live IPA URL forces dl=1" "dl=1"
        assert_header_value "live manifest is served as application/xml" "content-type" "application/xml"
    else
        skip "install happy path" "E2E_INSTALL_PATH not set"
    fi

    if [ -n "$E2E_APPINFO_PATH" ]; then
        request "$E2E_APPINFO_PATH"
        assert_status "live appinfo is served" "200"
        if python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$BODY" 2>/dev/null; then
            pass "live appinfo is valid JSON"
        else
            fail "live appinfo is valid JSON" "body did not parse"
        fi
    else
        skip "appinfo happy path" "E2E_APPINFO_PATH not set"
    fi
}


MODE="all"
for arg in "$@"; do
    case "$arg" in
        unit|e2e|all) MODE="$arg" ;;
        --no-build)   NO_BUILD=1 ;;
        --keep)       KEEP=1 ;;
        -h|--help)
            cat <<'USAGE'
Usage: ./Scripts/test.sh [unit|e2e|all] [--no-build] [--keep]

  unit        Run all Go test suites with the race detector
  e2e         Run Docker container smoke tests
  all         Run both suites (default)
  --no-build  Reuse the local test image
  --keep      Keep the disposable container after the smoke suite

Set E2E_SKIP_ONLINE=1 for offline smoke checks.
GO_BIN, E2E_IMAGE and E2E_PORT select the compiler, image and host port.
USAGE
            exit 0
            ;;
        *)            die "unknown argument: $arg" ;;
    esac
done

log ""
log "============================================================"
log " middleware test suite ($MODE)"
log "============================================================"

case "$MODE" in
    unit) run_unit_tests ;;
    e2e)  run_e2e_tests ;;
    all)  run_unit_tests && run_e2e_tests ;;
esac

section "Summary"
log "  ${GREEN}passed: $PASS_COUNT${NC}   ${RED}failed: $FAIL_COUNT${NC}   ${YELLOW}skipped: $SKIP_COUNT${NC}"
log ""
[ "$FAIL_COUNT" -eq 0 ] || exit 1
