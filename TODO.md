# TODO

Quester is buildable and usable now. Remaining work is optional hardening and product polish.

## Security and Deployment

- Add real authentication or fail-closed proxy integration before binding to anything other than loopback.
- Add per-request CSRF tokens if the app is ever served beyond a trusted local origin.
- Document a reverse-proxy deployment example with TLS, allowed hosts, and trusted proxy settings.

## Data and Operations

- Add a version field to the JSON task tree before future schema changes.
- Add a one-shot migration command for older `quester/*.json` data directories.
- Add periodic backup guidance or an export reminder for long-running installs.
- Consider file-locking if multiple Quester processes may share the same data directory.

## Product

- Add search across task titles and notes.
- Add sort options for created time, completion state, and title.
- Add due dates, priorities, or tags if the task model needs more structure.
- Add an undo or restore view for soft-deleted tasks.
- Add bulk actions for checking, deleting, or exporting selected tasks.

## Interface

- Do a pass with keyboard-only navigation and screen-reader labels.
- Add mobile screenshots to the README after the UI settles.
- Consider a small amount of progressive enhancement for instant toggle/add without losing the no-JavaScript fallback.

## Release

- Add release packaging once the install target is clear: Homebrew formula, container image, or downloadable binaries.
- Add a changelog when there is more than one public release.
