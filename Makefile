.PHONY: build clean ci race run test vet

ADDR ?= 127.0.0.1:93
DATA_DIR ?= .quester-data
PREFIX ?= /quester/

build:
	go build -o bin/quester .

clean:
	rm -rf bin coverage.out

ci: test vet race build

race:
	go test -race ./...

run:
	go run . -addr $(ADDR) -data-dir $(DATA_DIR) -prefix $(PREFIX)

test:
	go test ./...

vet:
	go vet ./...
