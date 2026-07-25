.PHONY: run lint test test-race build tidy fmt

run:
	go run ./cmd/server

lint:
	golangci-lint run ./...

test:
	go test ./... -count=1 -coverprofile=coverage.out

test-race:
	CGO_ENABLED=1 go test ./... -race -count=1 -coverprofile=coverage.out

build:
	go build ./...

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal
