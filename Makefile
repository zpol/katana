.PHONY: dev build test web-install web-build tidy certs fleet-dev fleet-build

GO ?= go
NPM ?= npm

dev:
	@mkdir -p data
	KATANA_AUTH_MODE="$(or $(KATANA_AUTH_MODE),local)" \
	KATANA_BOOTSTRAP_ADMIN_USER="$(or $(KATANA_BOOTSTRAP_ADMIN_USER),admin)" \
	KATANA_BOOTSTRAP_ADMIN_PASSWORD="$(or $(KATANA_BOOTSTRAP_ADMIN_PASSWORD),admin)" \
	KATANA_TOKEN="$(or $(KATANA_TOKEN),change-me-dev-token)" \
	KATANA_ADDR="$(or $(KATANA_ADDR),0.0.0.0:8080)" \
	KATANA_DB_PATH="$(or $(KATANA_DB_PATH),./data/katana.db)" \
	KATANA_ADMISSION_DRY_RUN="$(or $(KATANA_ADMISSION_DRY_RUN),true)" \
	$(GO) run ./cmd/katana

build: web-build
	@mkdir -p bin
	$(GO) build -o bin/katana ./cmd/katana

fleet-dev:
	@mkdir -p data
	PROMETHEUS_URL="$(or $(PROMETHEUS_URL),http://127.0.0.1:9090)" \
	FLEET_ADDR="$(or $(FLEET_ADDR),0.0.0.0:8090)" \
	FLEET_DB_PATH="$(or $(FLEET_DB_PATH),./data/fleet.db)" \
	$(GO) run ./cmd/fleet

fleet-build:
	@mkdir -p bin
	cd web-fleet && $(NPM) install && $(NPM) run build
	$(GO) build -o bin/fleet ./cmd/fleet

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

web-install:
	cd web && $(NPM) install --omit=optional

web-build:
	cd web && $(NPM) run build

certs:
	bash deploy/scripts/gen-webhook-certs.sh deploy/certs
