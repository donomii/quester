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

Everything in quester is a task in one tree: a "post" is a task under the
root, and a comment is a task under another task. Every task can carry file
attachments. Attach files while adding a comment (the reply carries the new
version) or with the attach form on a task's page. Every page belongs to
exactly one task — the one the `q` path in the URL ends at; the Open link on
a comment leads to that comment's own page, where the panel and forms act on
it.

Attachments sharing a file name are versions of one document. Which version
applies at a task is decided by walking up from that task, through its
parents, to the root: the first copy of that file name met on the way up is
the one in effect. So an attachment affects the task it is on and every task
below it, siblings never affect each other, and attaching a copy higher up
changes what applies everywhere below it except under tasks carrying their
own copy.

Version numbers count the copies of a name met along that same upward chain,
so two sibling comments can each show a "v2" that is a different file. The
short content id (a SHA-256 prefix) next to every version tells them apart —
equal ids mean identical bytes. Every comment in the tree carries a collapsed
"Documents in effect" line showing what applies at that comment, and every
entry in the documents panel links to a history page for that file name: the
copies met on the upward walk, and every copy attached on tasks below.

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
