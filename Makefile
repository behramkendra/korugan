.PHONY: build test vet fmt run tidy

build:
	go build -o bin/korugan ./cmd/korugan

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal

run: build
	./bin/korugan

tidy:
	go mod tidy
