GO ?= go
WEB_DIR := web
.PHONY: fmt test race vet build web-test web-typecheck web-build migrate seed verify
fmt:
	$(GO) fmt ./...
test:
	$(GO) test ./...
race:
	$(GO) test -race ./...
vet:
	$(GO) vet ./...
build:
	$(GO) build ./cmd/server
web-test:
	cd $(WEB_DIR) && npm test -- --run
web-typecheck:
	cd $(WEB_DIR) && npm run typecheck
web-build:
	cd $(WEB_DIR) && npm run build
migrate:
	./scripts/migrate.sh
seed:
	./scripts/seed.sh
verify: fmt test vet build
