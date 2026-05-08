BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
HOSTNAME ?= $(shell hostname -s)
ROOTDIR ?= $(shell git rev-parse --show-toplevel)
TEAMVAULT ?= ~/.teamvault.json
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/bborbe/dark-factory/pkg/version.Version=$(VERSION)

.PHONY: default
default: precommit

.PHONY: precommit
precommit: ensure format generate test check addlicense check-versions
	@echo "ready to commit"

.PHONY: check-versions
check-versions:
	@echo "Checking plugin version alignment..."
	@PLUGIN_VER=$$(python3 -c "import json; print(json.load(open('.claude-plugin/plugin.json'))['version'])"); \
	META_VER=$$(python3 -c "import json; print(json.load(open('.claude-plugin/marketplace.json'))['metadata']['version'])"); \
	PLUGINS0_VER=$$(python3 -c "import json; print(json.load(open('.claude-plugin/marketplace.json'))['plugins'][0]['version'])"); \
	echo "  plugin.json:                       $$PLUGIN_VER"; \
	echo "  marketplace.json metadata:         $$META_VER"; \
	echo "  marketplace.json plugins[0]:       $$PLUGINS0_VER"; \
	if [ "$$PLUGIN_VER" != "$$META_VER" ] || [ "$$PLUGIN_VER" != "$$PLUGINS0_VER" ]; then \
		echo "MISMATCH: 3 plugin JSON fields must equal each other (see CLAUDE.md → Version Alignment)"; \
		exit 1; \
	fi; \
	echo "Plugin versions aligned at $$PLUGIN_VER"

.PHONY: ensure
ensure:
	go mod tidy -e
	go mod verify
	rm -rf vendor

.PHONY: format
format:
	find . -type f -name 'go.mod' -not -path './vendor/*' -exec go run -mod=mod github.com/shoenig/go-modtool -w fmt "{}" \;
	find . -type f -name '*.go' -not -path './vendor/*' -exec gofmt -w "{}" +
	go run -mod=mod github.com/incu6us/goimports-reviser/v3 -project-name github.com/bborbe/dark-factory -format -excludes vendor ./...
	find . -type d -name vendor -prune -o -type f -name '*.go' -print0 | xargs -0 -P 8 -n 50 go run -mod=mod github.com/segmentio/golines --max-len=100 -w

.PHONY: generate
generate:
	rm -rf mocks avro
	mkdir -p mocks
	echo "package mocks" > mocks/mocks.go
	go generate -mod=mod ./...

.PHONY: test
test:
	go test -mod=mod -p=$${GO_TEST_PARALLEL:-1} -cover -race $(shell go list -mod=mod ./... | grep -v /vendor/)

.PHONY: check
check: lint vet errcheck vulncheck osv-scanner gosec trivy

.PHONY: lint
lint:
	go run -mod=mod github.com/golangci/golangci-lint/v2/cmd/golangci-lint run --allow-parallel-runners --config .golangci.yml ./...

.PHONY: vet
vet:
	go vet -mod=mod $(shell go list -mod=mod ./... | grep -v /vendor/)

.PHONY: errcheck
errcheck:
	go run -mod=mod github.com/kisielk/errcheck -ignore '(Close|Write|Fprint)' $(shell go list -mod=mod ./... | grep -v /vendor/ | grep -v k8s/client)

.PHONY: vulncheck
vulncheck:
	@go run -mod=mod golang.org/x/vuln/cmd/govulncheck -format json $(shell go list -mod=mod ./... | grep -v /vendor/) 2>&1 | \
		jq -e 'select(.finding != null and .finding.osv != "GO-2026-4923" and .finding.osv != "GO-2026-4514" and .finding.osv != "GO-2022-0470" and .finding.osv != "GO-2026-4772" and .finding.osv != "GO-2026-4771")' > /dev/null 2>&1 && \
		{ echo "Unexpected vulnerabilities found"; go run -mod=mod golang.org/x/vuln/cmd/govulncheck $(shell go list -mod=mod ./... | grep -v /vendor/); exit 1; } || \
		echo "No unignored vulnerabilities found"

.PHONY: osv-scanner
osv-scanner:
	@if [ -f .osv-scanner.toml ]; then \
		echo "Using .osv-scanner.toml"; \
		go run github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.3.1 --config .osv-scanner.toml --recursive .; \
	else \
		echo "No config found, running default scan"; \
		go run github.com/google/osv-scanner/v2/cmd/osv-scanner@v2.3.1 --recursive .; \
	fi

.PHONY: gosec
gosec:
	go run -mod=mod github.com/securego/gosec/v2/cmd/gosec \
	-exclude=G104,G115 \
	-quiet \
	-fmt=summary \
	-severity=medium \
	./...

.PHONY: trivy
trivy:
	trivy fs \
	--db-repository ghcr.io/aquasecurity/trivy-db \
	--scanners vuln,secret \
	--quiet \
	--no-progress \
	--disable-telemetry \
	--exit-code 1 .

.PHONY: addlicense
addlicense:
	go run -mod=mod github.com/google/addlicense -c "Benjamin Borbe" -y $$(date +'%Y') -l bsd $$(find . -name "*.go" -not -path './vendor/*')

.PHONY: install
install:
	go install -ldflags "$(LDFLAGS)" .

.PHONY: run
run:
	go run -ldflags "$(LDFLAGS)" main.go run

.PHONY: daemon
daemon:
	go run -ldflags "$(LDFLAGS)" main.go daemon
