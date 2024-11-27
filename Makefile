.PHONY: gomods
gomods: ## Install gomods
	go install github.com/jmank88/gomods@v0.1.3

.PHONY: gomodtidy
gomodtidy: gomods
	gomods tidy

.PHONY: mockery
mockery: $(mockery) ## Install mockery.
	go install github.com/vektra/mockery/v2@v2.46.3

.PHONY: rm-mocked
rm-mocked:
	grep -rl "^// Code generated by mockery" | grep .go$ | xargs -r rm

.PHONY: lint-workspace lint
GOLANGCI_LINT_VERSION := 1.62.2
GOLANGCI_LINT_COMMON_OPTS := --max-issues-per-linter 0 --max-same-issues 0
GOLANGCI_LINT_DIRECTORY := ./golangci-lint

lint-workspace:
	@./script/lint.sh $(GOLANGCI_LINT_VERSION) "$(GOLANGCI_LINT_COMMON_OPTS)" $(GOLANGCI_LINT_DIRECTORY)

lint:
	@./script/lint.sh $(GOLANGCI_LINT_VERSION) "$(GOLANGCI_LINT_COMMON_OPTS)" $(GOLANGCI_LINT_DIRECTORY) "--new-from-rev=origin/main"