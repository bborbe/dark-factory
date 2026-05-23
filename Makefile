include tools.env

SHELL := /bin/bash

BRANCH ?= $(shell git rev-parse --abbrev-ref HEAD | tr '/' '-')
HOSTNAME ?= $(shell hostname -s)
ROOTDIR ?= $(shell git rev-parse --show-toplevel)
TEAMVAULT ?= ~/.teamvault.json
VERSION ?= $(shell git describe --tags --dirty --always 2>/dev/null || echo "dev")
LDFLAGS := -X github.com/bborbe/dark-factory/pkg/version.Version=$(VERSION)

.PHONY: default
default: precommit

.PHONY: precommit
precommit: ensure format generate test check addlicense check-changelog check-links
	@echo "ready to commit"

.PHONY: check-links
check-links:
	@echo "Checking links in README.md and llms.txt..."
	@EXIT=0; \
	for file in README.md llms.txt; do \
		[ ! -f "$$file" ] && continue; \
		while read -r link; do \
			target=$${link%%#*}; \
			[ -z "$$target" ] && continue; \
			if [ ! -e "$$target" ]; then \
				echo "BROKEN: $$file -> $$link"; \
				EXIT=1; \
			fi; \
		done < <(grep -oE '\]\([^)]+\)' "$$file" 2>/dev/null | sed 's/^](//; s/)$$//' | grep -v '^http' | grep -v '^mailto:'); \
	done; \
	if [ "$$EXIT" -eq 1 ]; then exit 1; fi; \
	echo "All links OK"

.PHONY: check-changelog
check-changelog:
	scripts/check-changelog.sh

.PHONY: check-versions
check-versions:
	@bash scripts/check-versions.sh

.PHONY: release-check
release-check: precommit check-versions
	@echo "ready to release"

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
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --allow-parallel-runners --config .golangci.yml ./...

.PHONY: vet
vet:
	go vet -mod=mod $(shell go list -mod=mod ./... | grep -v /vendor/)

.PHONY: errcheck
errcheck:
	go run -mod=mod github.com/kisielk/errcheck -ignore '(Close|Write|Fprint)' $(shell go list -mod=mod ./... | grep -v /vendor/ | grep -v k8s/client)

VULNCHECK_IGNORE ?= GO-2026-4923 GO-2026-4514 GO-2022-0470 GO-2026-4772 GO-2026-4771

.PHONY: vulncheck
vulncheck:
	@PKGS="$(shell go list -mod=mod ./... | grep -v /vendor/)"; \
	IGNORE_JSON=$$(printf '%s\n' $(VULNCHECK_IGNORE) | jq -R . | jq -s .); \
	REMAIN=$$(go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) -format json $$PKGS 2>/dev/null | \
		jq -rs --argjson ignore "$$IGNORE_JSON" \
			'(map(select(.osv != null)) | map({key: .osv.id, value: (.osv.summary // "")}) | from_entries) as $$sum | \
			 map(select(.finding != null) | .finding) | \
			 map(select(.osv as $$o | $$ignore | index($$o) | not)) | \
			 map("\(.osv)\t\(.trace[-1].module)@\(.trace[-1].version) -> \(.fixed_version)\t\($$sum[.osv] // "")") | \
			 unique | .[]'); \
	if [ -n "$$REMAIN" ]; then \
		echo "Unexpected vulnerabilities (ignored: $(VULNCHECK_IGNORE)):"; \
		printf '%s\n' "$$REMAIN" | column -t -s "$$(printf '\t')"; \
		exit 1; \
	else \
		echo "No unignored vulnerabilities found"; \
	fi

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
	$(if $(wildcard .trivyignore),--ignorefile .trivyignore,$(if $(wildcard $(ROOTDIR)/.trivyignore),--ignorefile $(ROOTDIR)/.trivyignore,)) \
	$(if $(wildcard .trivy-secret.yaml),--secret-config .trivy-secret.yaml,$(if $(wildcard $(ROOTDIR)/.trivy-secret.yaml),--secret-config $(ROOTDIR)/.trivy-secret.yaml,)) \
	--scanners vuln,secret \
	--skip-dirs vendor \
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

.PHONY: fix
fix:
	@for dir in $$(find `pwd` -type d -name vendor -prune -o -name go.mod -exec dirname "{}" \; | grep -v '^$$'); do \
		cd $${dir}; \
		echo "fix $${dir}"; \
		go get github.com/go-git/go-git/v5@latest; \
		go get github.com/containerd/containerd@latest; \
		go get golang.org/x/crypto@latest; \
		go get golang.org/x/net@latest; \
	done
