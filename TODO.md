# TODO

Quester is buildable and usable now. Remaining work is optional hardening and product polish.

## Security and Deployment

- [x] Require a configured trusted authenticating proxy before binding beyond loopback.
- [x] Protect form mutations with per-request CSRF tokens bound to a same-site browser cookie.
- [x] Document a reverse-proxy deployment example with TLS, allowed hosts, and trusted proxy settings in `DEPLOYMENT.md`.

## Data and Operations

- Add a one-shot migration command for older `quester/*.json` data directories.
- Add periodic backup guidance or an export reminder for long-running installs.
- Consider file-locking if multiple Quester processes may share the same data directory.
- [x] Make backups self-contained: include `blobs/` content in downloadAll/restoreAll (zip archive).
- [x] Add garbage collection for blob files no longer referenced by any attachment record.

## Product

- [x] Add search across task titles and notes.
- [x] Add sort options for created time, completion state, and title.
- Add due dates, priorities, or tags if the task model needs more structure.
- [x] Add a restore view for soft-deleted tasks.
- Add inline previews for image attachments on the detail page.
- Add bulk actions for checking, deleting, or exporting selected tasks.

## Interface

- Do a pass with keyboard-only navigation and screen-reader labels.
- Add mobile screenshots to the README after the UI settles.
- Consider a small amount of progressive enhancement for instant toggle/add without losing the no-JavaScript fallback.

## Release

- Add release packaging once the install target is clear: Homebrew formula, container image, or downloadable binaries.
- Add a changelog when there is more than one public release.
