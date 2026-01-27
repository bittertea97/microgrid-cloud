SHELL := /bin/bash

COMPOSE_FILE ?= docker-compose.dev.yml
PG_DSN ?=

.PHONY: compose-up compose-down migrate e2e release-check

compose-up:
	docker compose -f $(COMPOSE_FILE) up -d

compose-down:
	docker compose -f $(COMPOSE_FILE) down -v

migrate:
	@if [[ -z "$(PG_DSN)" ]]; then echo "PG_DSN is required"; exit 1; fi
	@for f in $$(ls migrations/*.sql | sort); do psql "$(PG_DSN)" -f $$f; done

e2e:
	@bash scripts/pilot_e2e.sh
	@bash scripts/shadowrun_local.sh

release-check:
	@set -euo pipefail; \
	if [[ "$$OS" == "Windows_NT" ]]; then \
		if command -v pwsh >/dev/null 2>&1; then \
			pwsh -NoProfile -ExecutionPolicy Bypass -File scripts/release_acceptance.ps1; \
		else \
			echo "pwsh not found; install PowerShell 7 or run scripts/release_acceptance.sh in WSL/Git Bash"; \
			exit 1; \
		fi; \
	else \
		bash scripts/release_acceptance.sh; \
	fi
