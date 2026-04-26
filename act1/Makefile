ENVTEST_VERSION ?= 1.35.0
ENVTEST_OS      ?= linux
ENVTEST_ARCH    ?= amd64
KUBEBUILDER_ASSETS ?= $(HOME)/.local/share/kubebuilder-envtest/k8s/$(ENVTEST_VERSION)-$(ENVTEST_OS)-$(ENVTEST_ARCH)

.PHONY: setup-envtest build vet test-bugs test-bugs-assert-failures

setup-envtest:
	go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use $(ENVTEST_VERSION) --os $(ENVTEST_OS) --arch $(ENVTEST_ARCH)

build:
	go build ./...

vet:
	go vet ./...

test-bugs:
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./controller/... -v -timeout 60s || true

test-bugs-assert-failures:
	@FAIL_COUNT=$$(KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./controller/... -v -timeout 60s 2>&1 | grep -c '^--- FAIL:' || true); \
	echo "Failed tests: $$FAIL_COUNT (expected 3)"; \
	if [ "$$FAIL_COUNT" -ne 3 ]; then \
		echo "ASSERTION FAILED: expected exactly 3 test failures, got $$FAIL_COUNT"; \
		exit 1; \
	fi; \
	echo "ASSERTION PASSED: exactly 3 tests failed as expected"
