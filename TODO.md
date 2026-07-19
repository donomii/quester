# TODO

Quester is buildable and usable now. Remaining work is optional hardening and product polish.

## Security and Deployment

- [x] Require a configured trusted authenticating proxy before binding beyond loopback.
- [x] Protect form mutations with per-request CSRF tokens bound to a same-site browser cookie.
- [x] Document a reverse-proxy deployment example with TLS, allowed hosts, and trusted proxy settings in `DEPLOYMENT.md`.

## Data and Operations

- [x] Add a one-shot migration command for older `quester/*.json` data directories. `quester -migrate-from quester` validates and normalizes every JSON task tree into the unused `-data-dir` destination, leaves the source untouched, and exits without starting the server.
- [x] Show a backup reminder on the task summary and document a periodic off-host backup routine.
- [x] Serialize task-tree access with a data-directory file lock so separate Quester processes cannot lose one another's updates.
- [x] Make backups self-contained: include `blobs/` content in downloadAll/restoreAll (zip archive).
- [x] Add garbage collection for blob files no longer referenced by any attachment record.

## Product

- [x] Add search across task titles and notes.
- [x] Add sort options for created time, completion state, and title.
- [x] Add due dates, priorities, and searchable tags with validated input and migration defaults.
- [x] Add a restore view for soft-deleted tasks.
- [x] Add inline previews for safe raster image attachments on the detail page; active image formats such as SVG remain downloads.
- [x] Add bulk actions for checking, reopening, deleting, or exporting selected task trees and their attachment content.

## Interface

- [x] Add skip navigation, visible keyboard focus, accessible task selection and toggle names, page landmarks, and current-page state.
- Add mobile screenshots to the README after the UI settles.
- [x] Keep mutations as complete server-rendered form submissions: this preserves keyboard behavior and the no-JavaScript contract without maintaining a second client-side state path.

## Release

- Add release packaging once the install target is clear: Homebrew formula, container image, or downloadable binaries.
- Add a changelog when there is more than one public release.
