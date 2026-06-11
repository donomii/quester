# quester
A reddit style task tracker

Post your tasks like a reddit post and then comment on them to update them.

## Install

Check out the repository, then:

    go mod tidy
    go build -o bin/quester .
    ./bin/quester

## Use

Quester starts on `127.0.0.1:93`. Open:

    http://localhost:93/quester/

Task data is stored in `.quester-data/` by default. To use another location:

    ./bin/quester -data-dir /path/to/tasks

If you are upgrading from an older checkout that stored JSON files in `quester/`,
run:

    ./bin/quester -data-dir quester

## Configuration

Command-line flags:

    -addr 127.0.0.1:93
    -data-dir .quester-data
    -prefix /quester/

The same defaults can be overridden with `QUESTER_ADDR`, `QUESTER_DATA_DIR`,
and `QUESTER_PREFIX`.

Quester binds to loopback by default. Mutating form posts are rejected when they
come from a different browser origin.

## Development

Run the full local check:

    ./build.sh

Useful Make targets:

    make test
    make vet
    make race
    make build
    make run
    make ci
