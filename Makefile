APP     := flowgate
VERSION := $(shell cat VERSION 2>/dev/null || echo "0.0.0")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY  := $(shell git diff --quiet 2>/dev/null || echo "-dirty")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(GIT_COMMIT)$(GIT_DIRTY) \
	-X main.buildTime=$(BUILD_TIME)

.PHONY: build run test clean release tag bump-patch bump-minor bump-major help

build: ## Compile for the current platform
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o $(APP) ./cmd/flowgate

run: build ## Build and run locally
	./$(APP) --config config.yaml

test: ## Run all tests
	go test -count=1 -race ./...

clean: ## Remove build artifacts
	rm -f $(APP)
	rm -rf dist/

release: ## Cross-compile for all platforms (via build.sh)
	./scripts/build.sh

docker: ## Build Docker image
	docker build -t $(APP):$(VERSION) -t $(APP):latest .

tag: ## Create and push a git tag from VERSION
	git tag -a v$(VERSION) -m "Release v$(VERSION)"
	git push origin v$(VERSION)

bump-patch: ## Bump patch version (0.1.0 → 0.1.1)
	@V=$(VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	MINOR=$$(echo $$V | cut -d. -f2); \
	PATCH=$$(echo $$V | cut -d. -f3); \
	NEW="$$MAJOR.$$MINOR.$$((PATCH+1))"; \
	echo $$NEW > VERSION; \
	echo "Version bumped: $$V → $$NEW"

bump-minor: ## Bump minor version (0.1.0 → 0.2.0)
	@V=$(VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	MINOR=$$(echo $$V | cut -d. -f2); \
	NEW="$$MAJOR.$$((MINOR+1)).0"; \
	echo $$NEW > VERSION; \
	echo "Version bumped: $$V → $$NEW"

bump-major: ## Bump major version (0.1.0 → 1.0.0)
	@V=$(VERSION); \
	MAJOR=$$(echo $$V | cut -d. -f1); \
	NEW="$$((MAJOR+1)).0.0"; \
	echo $$NEW > VERSION; \
	echo "Version bumped: $$V → $$NEW"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
