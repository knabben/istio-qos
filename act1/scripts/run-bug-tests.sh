#!/usr/bin/env bash
# run-bug-tests.sh — run the Act I bug-exposure test suite and summarise results.
#
# Usage:
#   ./scripts/run-bug-tests.sh           # run and print results
#   ./scripts/run-bug-tests.sh --assert  # exit non-zero if exactly 3 tests don't fail
#
# Environment:
#   KUBEBUILDER_ASSETS  path to kube-apiserver + etcd binaries (set by setup-envtest)
#   ENVTEST_VERSION     envtest Kubernetes version to download (default: 1.35.0)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(dirname "$SCRIPT_DIR")"
ASSERT_MODE=false
ENVTEST_VERSION="${ENVTEST_VERSION:-1.35.0}"

for arg in "$@"; do
  case "$arg" in
    --assert) ASSERT_MODE=true ;;
    *) echo "Unknown argument: $arg" >&2; exit 1 ;;
  esac
done

# Resolve KUBEBUILDER_ASSETS if not already set.
if [[ -z "${KUBEBUILDER_ASSETS:-}" ]]; then
  echo "==> Resolving envtest binaries (version $ENVTEST_VERSION)…"
  KUBEBUILDER_ASSETS="$(
    cd "$MODULE_DIR"
    go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use "$ENVTEST_VERSION" -p path
  )"
  export KUBEBUILDER_ASSETS
fi

echo "==> KUBEBUILDER_ASSETS=$KUBEBUILDER_ASSETS"
echo "==> Running Act I bug-exposure tests (3 FAILs expected)…"
echo ""

cd "$MODULE_DIR"

# Run tests; capture output and ignore exit code (failures are expected).
OUTPUT="$(KUBEBUILDER_ASSETS="$KUBEBUILDER_ASSETS" go test ./controller/... -v -timeout 60s 2>&1 || true)"
echo "$OUTPUT"

echo ""
echo "==> Summary"

PASS_COUNT=$(echo "$OUTPUT" | grep -c '^--- PASS:' || true)
FAIL_COUNT=$(echo "$OUTPUT" | grep -c '^--- FAIL:' || true)

echo "    PASS: $PASS_COUNT"
echo "    FAIL: $FAIL_COUNT"

if "$ASSERT_MODE"; then
  if [[ "$FAIL_COUNT" -ne 3 ]]; then
    echo "==> ASSERTION FAILED: expected exactly 3 failures, got $FAIL_COUNT"
    exit 1
  fi
  echo "==> ASSERTION PASSED: exactly 3 tests failed (bugs confirmed), 1 passed (happy path ok)"
fi
