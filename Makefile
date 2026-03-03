.PHONY: help fmt fmt-check vet test tidy

help:
	@echo "Targets:"
	@echo "  fmt        Run gofmt on all Go files"
	@echo "  fmt-check  Verify gofmt has no changes"
	@echo "  vet        Run go vet"
	@echo "  test       Run unit tests"
	@echo "  tidy       Run go mod tidy"

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to be run on:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

test:
	go test ./...

tidy:
	go mod tidy
