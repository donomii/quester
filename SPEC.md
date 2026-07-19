# Quester behavior specification

## Purpose

Quester is a private, browser-based task tracker arranged like a forum. A top-level post is a tracked task. Replies can be conversation entries or tracked subtasks. Every node can contain notes and immutable attachment records. Task data and attachment content stay in a user-selected local data directory.

## Startup and configuration

The program accepts these settings, with environment variables providing the editable defaults shown below:

- listen address: `QUESTER_ADDR`, default `127.0.0.1:93`;
- data directory: `QUESTER_DATA_DIR`, default `.quester-data`;
- URL prefix: `QUESTER_PREFIX`, default `/quester/`;
- trusted authenticating proxies: `QUESTER_TRUSTED_PROXIES`, default empty on loopback;
- legacy migration source: `-migrate-from`, default empty;
- self-check: `-test`, default off.

A non-loopback listen address is rejected unless at least one exact proxy address or network is configured. When proxies are configured, every request must come from one and must carry its authenticated identity. The migration command validates and normalizes all source JSON files into an unused destination, leaves the source untouched, and exits. The self-check validates core model, migration, and template behavior and exits.

## Stored data

Each authenticated workspace has one JSON task tree. The root carries schema version 2, forums, users, attachments, and child nodes. A node contains:

- stable identifier, title, notes, forum identifier, and author identifier;
- creation and update times;
- whether it has open/done state, whether it is done, and whether it is soft-deleted;
- optional due date in `YYYY-MM-DD` form;
- priority: `low`, `normal`, `high`, or `urgent`, defaulting to `normal`;
- up to 20 case-insensitively unique tags, each no longer than 40 characters;
- attachments and child nodes.

Older task trees receive the `normal` priority default during normalization. Invalid dates, priorities, tags, JSON, and attachment content addresses are rejected rather than replaced with synthesized values.

An attachment contains its stable identifier, safe file name, SHA-256 blob reference, byte size, attachment time, and optional identifier of the revision it replaces. Blob bytes live once per content hash under `<data-directory>/blobs/`.

Task-tree reads and writes take both an in-process lock and an advisory data-directory lock. Separate Quester processes sharing a directory therefore serialize updates instead of overwriting one another. Blob writes are content-addressed and atomically installed.

## Browser interaction

Every page provides primary navigation, a keyboard skip link, visible keyboard focus, page landmarks, current-forum state, and descriptive form labels. All mutations work as ordinary server-rendered forms without JavaScript and require a same-origin request plus a token bound to a same-site browser cookie.

The summary page lists visible top-level tasks in the selected forum. It can show all or open tasks and sort newest-first, open-first, or by title. Each task shows status, author, time, priority, optional due date, optional tags, reply count, and attachment count. The page also reminds the user to download and retain an off-host backup.

The search page finds case-insensitive matches in titles, notes, or tags. The deleted page lists soft-deleted nodes and restores them. Restoring a child beneath a deleted parent does not make it visible until its parent is restored.

The detail page shows the selected node, its visible replies, metadata, documents in effect, and forms to reply, attach, edit, move, promote, or delete. New and edited tasks expose the due date, priority, and comma-separated tag settings together with explanations. Safe raster image attachments receive bounded lazy-loaded previews. SVG and other active or unknown formats remain download-only.

Summary and search results have labeled selection checkboxes. A user can mark all selected nodes done, mark them open, soft-delete them, or download a selected-task zip. A selected parent includes its subtree once even if descendants are also selected. Invalid or empty selections do not partially change data.

## Forums, replies, and movement

A top-level post requires a title and belongs to an existing forum. A reply inherits its parent's forum and may omit its title. Promoting a reply to the top level requires a title and forum. Moving beneath another node adopts that node's forum. A node cannot move beneath itself or one of its descendants.

Deleting a node is soft deletion and hides its subtree from normal pages. The root cannot be deleted. Open/done changes set task tracking on and update the modification time.

## Attachments and revisions

Uploads are limited to 100 MB of combined content per request. A revision relationship exists only when the user explicitly selects the attachment being replaced; matching file names do not imply revision. The document view serves safe text, PDF, raster images, video, and audio inline. HTML, SVG, unknown, and potentially active formats download as attachments. All responses prevent content-type guessing.

Document state follows the path from the root to the current node. The deepest revision in each explicit revision family is in effect. Sibling branches remain independent. History shows the chain to the selected revision and later revisions descended from it.

## Backup, restore, and cleanup

Full backup downloads a zip named `quester-backup.zip` containing `tasks.json` and every referenced blob. Selected-task export uses the same format with only the selected task trees and their referenced blobs. Restore accepts a self-contained zip or a legacy task JSON file. Every restored blob must hash to the name recorded in the archive before the task tree is replaced.

The cleanup page first lists unreferenced blob files older than one hour. Cleanup recomputes the list under the data lock and removes only files still unreferenced. Soft-deleted nodes retain their blob references. Operators should keep multiple dated backups outside the Quester host and periodically verify restoration into an unused directory.

## Private API

The JSON API lists forums, users, forum nodes, individual nodes, changes since an RFC3339 time, and mentions of a user. It creates forums and nodes, updates node status, moves nodes, and attaches documents. Node JSON includes due date, priority, and tags. Node creation accepts those fields with the same validation as the browser forms. JSON bodies reject unknown fields and trailing values. API errors identify the invalid field or missing object without exposing stored content.
