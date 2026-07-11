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

A comment in quester is itself a task — same record, same forms, same page.
The only relationship is "replies to": every comment replies to exactly one
task, and a post replies to the front page, which is also a task (named
Quester — a file attached there applies to everything). Every task can carry
file attachments. Attach files while adding a comment (the reply carries the
new version) or with the attach form on a task's page. Every page belongs to
exactly one task — the one the `q` path in the URL ends at; the Open link on
a comment leads to that comment's own page, where the panel and forms act on
it.

Attachments sharing a file name are versions of one document. To find the
version in effect at a task: look at the task itself, then at the task it
replies to, and so on toward the front page; the first copy of that name
found wins (the newest one, if a task carries several). So an attachment
affects the task it is on and every reply under it, parallel replies never
affect each other, and attaching a copy nearer the post changes what applies
for every reply under it except those carrying their own copy.

Version numbers count the copies of a name along that same chain of replies,
so two parallel comments can each show a "v2" that is a different file. The
short content id (a SHA-256 prefix) next to every version tells them apart —
equal ids mean identical bytes. Every comment carries a collapsed "Documents
in effect" line showing what applies at that comment, and every entry in the
documents panel links to a history page for that file name: the copies found
along the reply chain, and every copy attached on replies below.

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
