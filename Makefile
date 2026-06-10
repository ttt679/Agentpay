.PHONY: test build lint fmt tidy

# Run all tests
test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Build the project
build:
	go build -v ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -w .

# Tidy dependencies
tidy:
	go mod tidy

# Run example
example:
	cd examples && go run gin_example.go
