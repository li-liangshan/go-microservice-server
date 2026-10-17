BINARY=bin/server
PKG=./...

.PHONY: build run test fmt vet tidy clean

build:
	go build -o $(BINARY) ./cmd/server

run: build
	./$(BINARY) --config configs/config.yaml

test:
	go test $(PKG) -v -race -cover

fmt:
	go fmt $(PKG)

vet:
	go vet $(PKG)

tidy:
	go mod tidy

clean:
	rm -rf bin
