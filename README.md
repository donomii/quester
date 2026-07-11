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

## Attachments

Any task or comment can carry file attachments. Attach files while adding a
comment (the reply carries the new version) or with the attach form on a
task's own page. Every page belongs to exactly one task — the one the `q`
path in the URL ends at; the Open link on a comment leads to that comment's
own page, where the panel and forms act on it.

Attachments sharing a file name are versions of one document. The detail page
lists the documents in effect for the task you are viewing: the deepest
attachment along the path from the root wins, so a version attached on a
comment overrides versions attached above it — for that branch only.
Re-attaching at a higher level re-baselines every branch that has not
overridden the name.

Version numbers count along a branch, so sibling branches can each carry
their own v2 of a file. The short content id (a SHA-256 prefix) next to
every version is what tells parallel versions apart — equal ids mean
identical bytes. Every comment in the tree also carries a collapsed
"Documents in effect" line showing the resolved set at that point.

File content is stored once per unique file (SHA-256 content addressing) in
`<data-dir>/blobs/`. The JSON backup from Backup/Restore carries attachment
records but not file content; back up the whole data directory to keep
attachments.

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
