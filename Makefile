.PHONY: golangci-lint
golangci-lint: ## Run golangci-lint for all issues.
	[ -d "./golangci-lint" ] || mkdir ./golangci-lint && \
	docker run --rm -v $(shell pwd):/app -w /app golangci/golangci-lint:v1.59.1 golangci-lint run --max-issues-per-linter 0 --max-same-issues 0 | tee ./golangci-lint/$(shell date +%Y-%m-%d_%H:%M:%S).txt

