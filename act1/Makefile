ENVTEST_VERSION ?= 1.35.0
ENVTEST_OS      ?= linux
ENVTEST_ARCH    ?= amd64
KUBEBUILDER_ASSETS ?= $(HOME)/.local/share/kubebuilder-envtest/k8s/$(ENVTEST_VERSION)-$(ENVTEST_OS)-$(ENVTEST_ARCH)

IMAGE_TAG ?= podlabeler:act1
KIND_CLUSTER ?= istio-qos

.PHONY: setup-envtest build vet test-bugs test-bugs-assert-failures docker-build kind-load cluster-deploy cluster-teardown

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

docker-build:
	docker build -t $(IMAGE_TAG) .

kind-load:
	kind load docker-image $(IMAGE_TAG) --name $(KIND_CLUSTER)

cluster-deploy:
	kubectl apply -f manifests/crd.yaml
	kubectl apply -f manifests/rbac.yaml
	kubectl apply -f manifests/controller.yaml
	kubectl apply -f manifests/sample-policies.yaml
	kubectl apply -f manifests/sample-workload.yaml

cluster-teardown:
	kubectl delete -f manifests/sample-workload.yaml --ignore-not-found
	kubectl delete -f manifests/sample-policies.yaml --ignore-not-found
	kubectl delete -f manifests/controller.yaml --ignore-not-found
	kubectl delete -f manifests/rbac.yaml --ignore-not-found
	kubectl delete -f manifests/crd.yaml --ignore-not-found
