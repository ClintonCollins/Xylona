# OPS-02 Backups Design

Date: 2026-04-06
Status: Approved for planning
Backlog item: `OPS-02`

## Summary

Add a server-level backup feature with:

- manual backup creation
- scheduled backups through the existing Scheduled Tasks UI
- backup history browsing
- restore to an existing server in either `overlay` or `exact` mode
- count-based retention for automated backups only

The backup artifact for v1 is the game server directory only. It does not attempt to snapshot or reconstruct Xylona database records beyond the existing server entry.

V1 is local-node only. Remote or federated server backups are explicitly out of scope until the repo has a real backup dispatch model.

## Validated Decisions

- Backup contents: server directory only
- Restore target: the existing server only
- Restore mode: user chooses `overlay` or `exact` at restore time
- Live backup behavior: do not stop the server before creating a backup
- Retention: simple count-based retention
- Manual backups: allowed and never auto-pruned
- Scheduling model: scheduled backups use the existing Scheduled Tasks form and page
- Backup UX: dedicated `Backups` tab for manual backup, history, restore, and settings
- `backups_enabled`: master feature gate for both manual and scheduled backups
- `backups_enabled` visibility: only superusers can view or modify the master toggle
- New server default: `backups_enabled = true` for now
- Disabled state UX: the `Backups` tab remains visible and shows a disabled alert state

## Goals

- Give users a clear way to create and restore server backups from the UI
- Reuse the existing scheduler instead of inventing a second scheduling subsystem
- Keep retention simple for v1
- Preserve manual backups until a user explicitly deletes them
- Prevent restore behavior from deleting or overwriting anything outside the target server directory

## Non-Goals

- Backing up Xylona database state or reconstructing server rows from a backup
- Selective file or folder restore
- Restoring into a new cloned server
- Tiered retention like daily and weekly buckets
- Cloud storage targets or remote backup sync
- Remote or federated backup management
- Game-definition-controlled backup defaults
- Embedding a second scheduler UI inside the `Backups` tab

## User Experience

### Backups Tab

Add a dedicated `Backups` tab to the game server layout. It is gated by `game_server.backup` and is shown only for local servers in v1.

When backups are enabled, the tab contains:

- a manual `Create Backup` action
- a backup settings section for superusers
- a backup history table
- restore and delete actions per backup
- an `Automated Backups` section with a shortcut to the `Schedules` tab

When backups are disabled, the tab still renders, but the main state is a prominent alert:

`Backups are disabled for this server.`

When disabled:

- non-superusers see the disabled state only
- superusers also see the master toggle so they can re-enable the feature
- manual backup, restore, and delete controls are hidden or disabled
- the automated backup shortcut is hidden or disabled
- existing backup history may still be shown read-only for context

### Backup Settings

The settings surface in the `Backups` tab is superuser-only and includes:

- `Enable Backups`
- `Backup Directory`
- `Max Automated Backups`

Behavior:

- only superusers can view or edit `Enable Backups`
- only superusers can view or edit `Backup Directory`
- only superusers can view or edit `Max Automated Backups`
- `Backup Directory` and `Max Automated Backups` are only meaningful when backups are enabled
- for v1, newly created servers default to `backups_enabled = true`
- for v1, newly created servers default `max_backups` to a positive value, recommended `10`
- the future game-definition default is explicitly deferred

The current database defaults are not sufficient for the desired behavior. The server creation path should explicitly initialize new servers with:

- `backups_enabled = true`
- a valid node-local `backup_directory`
- a positive `max_backups`, recommended `10`

The feature must not create a new server that is enabled for backups but left with an empty backup directory or a zero-retention configuration.

Legacy normalization rule:

- existing rows with `backups_enabled = true` and `max_backups = 0` must be normalized before scheduled backups can run
- preferred approach: migrate them to the chosen default, recommended `10`
- minimum acceptable fallback: block scheduled backup creation and execution until a superuser saves a positive value

### Backup History

The history table should show:

- creation time
- trigger source: `manual` or `scheduled`
- status: `pending`, `completed`, `failed`
- archive size
- archive file name or short artifact label
- retention exemption indicator for manual backups
- row actions

Row actions:

- `Restore`
- `Delete`

Manual backups should be visibly marked as retention-exempt so users understand why they are not auto-pruned.

### Restore Flow

Restore is always targeted at the current server's directory and requires the server to be offline first.

The restore dialog asks the user to choose one of two modes:

- `Overlay Restore`: extract the backup on top of the current directory and keep extra files that are not present in the backup
- `Exact Restore`: make the current directory match the backup exactly, including deleting files that are not present in the backup

The dialog should explain the tradeoff clearly:

- `Overlay` is safer when the user wants to preserve new files
- `Exact` is the closest point-in-time restore but is destructive inside the server directory

### Automated Backups Shortcut

The `Backups` tab should include a lightweight `Automated Backups` section.

This section does not embed a second scheduler form. Instead it provides:

- backup schedule summary if one or more scheduled backup tasks exist
- `Create Scheduled Backup` when none exist
- `Manage Scheduled Backups` when one or more exist

The shortcut should deep-link to the `Schedules` tab. If practical, it should open the create form with task type preselected to `backup`, or filter the list to backup tasks.

Because scheduled tasks are local-only today, this shortcut is also local-only in v1.

## Scheduling Model

Scheduled backups are owned by the existing Scheduled Tasks system.

### Scheduled Task Integration

Add a new scheduled task type: `backup`.

The existing Scheduled Task form and page remain the only UI for creating and editing scheduled backup runs.

Scope:

- supported for local servers only in v1
- not exposed for remote or federated servers until a real dispatch path exists

Permission model:

- the `Schedules` tab still requires `game_server.scheduled_tasks`
- scheduled backup creation additionally requires `game_server.backup`

Feature gate model:

- if `backups_enabled` is false, new `backup` scheduled tasks cannot be created
- existing scheduled backup tasks may still be listed and deleted
- if an enabled scheduled backup task remains after backups are disabled, execution logs a clear failure and produces no artifact

This keeps scheduling in one place while allowing the `Backups` tab to remain the source of truth for backup history and restore.

## Architecture

### Backup Service

Add a dedicated backup service in the actions layer, likely `actions/backup_service.go`.

Primary responsibilities:

- validate backup configuration and permissions
- create backup catalog rows
- archive the server directory
- list and delete backup artifacts
- restore an artifact into the server directory
- prune automated backups after successful scheduled runs

Manual backups and scheduled backups both call the same backup service. The only behavioral difference is the trigger source and retention exemption flag.

### Archive and Extract Implementation

Do not wrap the current file-browser archive RPC helpers directly. Those helpers are intentionally constrained to archives that live under `gameServer.Directory`, which does not fit the proposed backup storage model.

Instead, create a backup-specific lower-level archive and restore implementation that reuses the same underlying libraries and safety patterns, while being able to:

- read and write backup archives outside the server root
- extract into a staging directory outside the live server tree
- restore executable and base-command files that may not fit the current file-browser path assumptions

It should still borrow the good parts of the existing file action code:

- path validation behavior consistent
- progress reporting behavior consistent
- archive-format support centralized

For v1, the backup feature should pick a single archive format for predictability. ZIP is the safest default because it is easy to inspect across platforms and already fits the existing archive libraries well.

### Storage Layout

`backup_directory` remains a server-level destination root, but it is a superuser-managed node-local path. Backups should be stored under a per-server subdirectory to avoid collisions and keep pruning simple.

For v1, new servers should get a valid default backup path derived from the server's local storage location and placed outside the server directory. A sibling backup directory is preferred over storing backups inside the server tree, which would otherwise risk recursive archive growth.

Suggested artifact layout:

`<backup_directory>/<game_server_id>/<timestamp>-<source>.zip`

Example:

`/srv/xylona-backups/server-123/2026-04-06T14-35-00Z-manual.zip`

The catalog should store the exact resolved archive path plus the node identity that owns the artifact.

## Data Model

### Existing `game_server` Fields

Retain the existing columns and clarify their semantics:

- `backups_enabled`
  - master feature gate for all backup functionality on the server
  - if false, no manual or scheduled backup run is allowed
- `backup_directory`
  - required node-local destination root for backup artifacts when backups are enabled
  - superuser-managed because it grants host filesystem write and delete authority
- `max_backups`
  - number of completed, non-exempt scheduled backups to keep
  - does not apply to manual backups
  - must be a positive integer for new configured servers in v1

### New `game_server_backup` Table

Add a new table to catalog backup artifacts.

Suggested fields:

| Column | Purpose |
| --- | --- |
| `id` | Primary key |
| `game_server_id` | Owning server |
| `node_id` | Node that owns the artifact |
| `created_by` | User who initiated the backup when applicable |
| `trigger_source` | `manual` or `scheduled` |
| `archive_path` | Absolute path to the stored archive |
| `archive_format` | `zip` for v1 |
| `status` | `pending`, `completed`, `failed` |
| `size_bytes` | Final artifact size |
| `retention_exempt` | `true` for manual backups |
| `error_message` | Failure detail when status is `failed` |
| `created_at` | Row creation time |
| `completed_at` | Completion time |

Rationale:

- backup history should not be inferred from files on disk
- retention decisions need metadata
- restore should target known artifacts, not directory scans
- node-local artifacts need node affinity for safe restore and delete

V1 node behavior:

- backups are node-local artifacts
- restore and delete are only supported when the backup row `node_id` matches the current server node
- cross-node restore is out of scope
- if a server moves nodes later, old backups remain historical records but are not restorable in v1 unless the operator also migrates the artifact and metadata manually

No additional restore-history table is required for v1.

## RPC Design

Add dedicated backup RPCs for backup-specific operations:

- `GetGameServerBackupOverview`
- `GetBackupSettings`
- `UpdateBackupSettings`
- `CreateGameServerBackup`
- `ListGameServerBackups`
- `DeleteGameServerBackup`
- `RestoreGameServerBackup`

Responsibilities:

- `GetGameServerBackupOverview` is available to callers with `game_server.backup`
  - returns whether backups are currently available for operations
  - returns disabled-state messaging needed by the `Backups` tab
  - does not expose raw superuser-only settings unnecessarily
- backup settings RPCs manage `backups_enabled`, `backup_directory`, and `max_backups`
  - `GetBackupSettings` and `UpdateBackupSettings` are superuser-only
- history RPC lists artifacts from `game_server_backup`
- manual backup RPC creates a history row and starts the archive workflow
- restore RPC validates the server state and applies the selected restore mode

Scheduled backups do not get a separate scheduling RPC surface. They continue to use the existing scheduled task RPCs after `backup` becomes a valid task type.

Existing generic game-server detail and list responses must not remain the source of truth for superuser-only backup settings. The implementation needs either field redaction or a shift to dedicated backup overview and settings RPCs for backup UI state.

## Progress Reporting

Manual backup creation and restore should expose progress to the frontend.

The progress model should reuse the same style as existing archive and extract operations rather than inventing a completely different shape.

Two reasonable implementation options are acceptable:

- connect streaming progress on create and restore RPCs
- websocket progress events backed by a short-lived task id

The implementation plan can choose between them based on how much code reuse is easiest with the current RPC layer. The important part is that the user can see in-flight progress for long-running archive and restore work on local servers.

## Backup Execution Flow

### Manual Backup

1. Validate `game_server.backup` permission.
2. Validate `backups_enabled` is true.
3. Validate `backup_directory` is configured.
4. Validate the target server is local to the node handling the request.
5. Create a `game_server_backup` row in `pending`.
6. Archive the entire server directory into the configured backup location.
7. Mark the row `completed` with size and completion time, or `failed` with an error.

### Scheduled Backup

1. Scheduler executes a scheduled task with `task_type = backup`.
2. Scheduler calls the same backup service used by manual backups.
3. The backup service records the artifact with `trigger_source = scheduled` and `retention_exempt = false`.
4. On successful completion, prune old scheduled backups beyond `max_backups`.
5. Scheduled task execution logs remain in the existing scheduled-task log table.
6. Artifact history remains in `game_server_backup`.

### Retention Rules

Retention applies only to:

- completed backups
- non-exempt backups
- scheduled backups

Retention does not apply to:

- manual backups
- failed backups
- pending backups

Pruning rule:

- after a successful scheduled backup, keep the newest `max_backups` completed scheduled backups for that server
- delete older completed scheduled backups, oldest first

## Restore Flow

### Preconditions

Restore is allowed only when:

- the caller has `game_server.backup`
- `backups_enabled` is true
- the backup artifact exists
- the backup row `node_id` matches the current server node
- the server is offline

### Safe Restore Sequence

1. Validate the backup record and archive file.
2. Extract the archive into a staging directory.
3. Validate every extracted path.
4. Apply either `overlay` or `exact` to the target server directory.
5. Report success or failure to the user.

Why staging first:

- avoids partially replacing the live server directory while the archive is still being read
- makes it possible to compute the set difference needed for `exact` restore
- gives one place to reject path traversal or malformed archive entries before destructive work begins

### Restore Modes

`overlay`:

- copy extracted files onto the server directory
- do not delete extra existing files

`exact`:

- make the server directory match the extracted backup exactly
- remove files and directories inside the server directory that are not present in the extracted backup

`exact` is the only intentionally destructive path in this feature, and it must be strictly constrained to the target server directory after full path validation.

## Permissions

### Backup Operations

- `game_server.backup` is required for:
  - viewing the `Backups` tab
  - creating a manual backup
  - listing backup history
  - deleting a backup
  - restoring a backup

### Master Toggle

- only superusers can view or change `backups_enabled`
- only superusers can view or change `backup_directory`
- only superusers can view or change `max_backups`
- non-superusers can still use backup operations when backups are enabled and they have `game_server.backup`
- non-superusers may learn that backups are unavailable through the backup overview state, but they do not get the raw settings controls

### Scheduled Backups

- `game_server.scheduled_tasks` is required to use the `Schedules` tab
- `game_server.backup` is additionally required to create or edit a `backup` scheduled task
- v1 scheduled backup support is local-only

## Error Handling

Backup creation should fail fast with clear user-facing errors for:

- backups disabled
- missing `backup_directory`
- missing server
- missing or invalid artifact path
- request targets a remote or federated server in v1
- archive creation failure

Restore should fail fast with clear user-facing errors for:

- backups disabled
- server not offline
- backup record missing
- archive file missing
- backup row belongs to a different node than the current server
- unreadable or corrupt archive
- path traversal entry in the archive
- any extracted path resolving outside the server directory

Operational safeguards:

- staging extraction is required before apply
- `exact` deletion is allowed only inside the server directory
- pruning deletes only cataloged backup artifacts for the current server

## Testing Strategy

### Backend Unit Tests

- backup service manual happy path
- scheduled backup happy path
- disabled-backup rejection
- missing `backup_directory` rejection
- retention pruning ignores manual backups
- retention pruning ignores failed backups
- restore `overlay` behavior
- restore `exact` behavior
- corrupt archive handling
- missing archive handling
- path traversal rejection
- scheduler execution of `backup` task type

### RPC Tests

- `game_server.backup` enforcement for backup operations
- superuser-only access to `backups_enabled`
- scheduled backup task creation requiring both scheduler and backup permissions
- disabled-state errors for manual and scheduled backup execution
- node-mismatch rejection

### Frontend Tests

- `Backups` tab visibility when user has `game_server.backup`
- disabled alert rendering when backups are off
- superuser rendering of the master toggle
- non-superuser read-only disabled state
- backup history row rendering
- restore mode selection dialog
- automated backup shortcut routing to `Schedules`
- scheduled task form rendering the new `backup` type
- local-only visibility for the `Backups` tab and backup scheduled task type

## Implementation Touchpoints

### Likely New Files

- `actions/backup_service.go`
- `actions/backup_service_test.go`
- `db/game-server-backup.go`
- `db/game-server-backup_test.go`
- `frontend/src/pages/game_servers/GameServerBackups.vue`
- `frontend/src/components/game_servers/BackupRestoreDialog.vue`

### Likely Modified Files

- `sql/migrations/*` for the new backup table and any necessary data updates
- `proto/shared.proto`
- `proto/xylona.proto`
- `api/rpc/scheduled-task.go`
- `api/rpc/game-server-dispatch.go` or equivalent dispatch/visibility helpers
- `pkg/scheduler/executor.go`
- `db/scheduled-task.go`
- `frontend/src/components/game_servers/ScheduledTaskForm.vue`
- `frontend/src/pages/game_servers/GameServerSchedules.vue`
- `frontend/src/pages/game_servers/GameServerLayout.vue`
- `frontend/src/pages/game_servers/game-server-layout-tabs.ts`

Generated outputs would be regenerated during implementation and are not to be hand-edited.

## Deferred Follow-Ups

- game-definition-level default for `backups_enabled`
- tiered retention policies
- clone restore into a new server
- selective restore
- backup downloads from the UI
- remote or cloud backup destinations
- restore history or audit log integration
