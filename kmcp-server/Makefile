PYTHON      ?= python3
PIP         ?= pip3
RBAC_DIR    := rbac
SERVER      := server.py

.PHONY: setup install-rbac start stop test

setup:
	$(PIP) install -r requirements.txt

install-rbac:
	kubectl apply -f $(RBAC_DIR)/role.yaml
	kubectl apply -f $(RBAC_DIR)/rolebinding.yaml

start:
	$(PYTHON) $(SERVER)

stop:
	pkill -f "$(SERVER)" || true

test:
	pytest tests/ -v --tb=short
