VERSION ?= dev

.PHONY: build test lint

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/dctop .

test:
	go test ./...

lint:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	go vet ./...
