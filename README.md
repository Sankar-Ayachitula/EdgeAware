# EdgeAware Backend

Go backend for managers and field operators: a multi-level hierarchy of users,
tasks assigned down that hierarchy, and offline sync for field devices.

Stack: Go standard-library router, GORM on PostgreSQL, JWT auth, bcrypt passwords.

The operator mobile app (Kotlin Multiplatform, Android and iOS) is in [mobile/](mobile/README.md).

<p align="center">
  <img src="mobile/docs/demo.gif" alt="Operator app: syncing, changing a task's status, filters, and the bottom tabs" width="300">
</p>

## Run locally

```bash
docker compose up -d          # Postgres on :5432 (skip if you already run Postgres)
go run ./cmd/seed             # creates tables + a sample 4-level org (empty DB only)
JWT_SECRET=change-me go run ./cmd/api
```

The seeded users and their shared development password are in
[internal/seed/seed.go](internal/seed/seed.go).

Environment: `DATABASE_URL` (or `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`),
`JWT_SECRET`, `TOKEN_TTL` (default `15m`), `PORT` (default `8080`),
`CLEANUP_INTERVAL` (default `24h`, `0` disables), `FEED_RETENTION` (default `720h`),
`OPS_RETENTION` (default `168h`).

## How the hierarchy works

| Table | Purpose |
|---|---|
| `roles` | Name + numeric `level`. 0 = field operator; higher = more senior. |
| `users` | Each user has a `role_id` and a `manager_id` (their direct boss). |
| `user_hierarchy` | Closure table: one row for every (senior, junior) pair at every depth. |
| `tasks` | `assigned_to`, `created_by`, status, due date, `completed_at`, `version`. |
| `sync_feed` | Per-user change log for offline devices (see below). |
| `devices` | Each syncing device, its owner, last seen time and last pulled cursor. |
| `processed_ops` | Results of pushed changes, so retries are not applied twice. |
| `sync_watermarks` | Per user, the highest feed entry the cleanup job has deleted. |

Access rules (enforced in [internal/policy](internal/policy/policy.go)):

- You can see yourself and everyone **below** you. Never your seniors or another branch.
- You can edit, reassign and delete tasks of people below you.
- An assignee can only change the **status** of tasks given to them by a senior.
- You can add users under yourself or anyone below you; their role level must be lower than their manager's.
- A user's manager must always have a higher role level, and loops are rejected.

The logged-in user always comes from the JWT; roles and hierarchy are read from the
database on every request, so changes apply immediately.

## API

All routes except `/health` and `/login` need `Authorization: Bearer <token>`.

| Method & path | Description |
|---|---|
| `POST /login` | `{username, password}` → `{access_token}` |
| `GET /me` | Logged-in user with role |
| `GET /roles` | All roles by level |
| `GET /users/{id}` | A user you can see |
| `GET /users/{id}/juniors` | Direct juniors (the tabs on the hierarchy page) |
| `GET /users/{id}/team` | Everyone below that user |
| `POST /users` | `{username, password, role_id, manager_id}` |
| `PATCH /users/{id}/manager` | `{manager_id}` — moves the user and their team |
| `GET /users/{id}/tasks?status=` | Tasks assigned to that user |
| `POST /tasks` | `{title, description?, due_at?, assigned_to?, id?}` (`id` lets offline clients pick the UUID) |
| `GET /tasks/{id}` | One task |
| `PATCH /tasks/{id}` | Partial update; must include the current `version`, else `409` with the server copy |
| `DELETE /tasks/{id}?version=` | Soft delete (optional version check) |
| `GET /sync/pull?since=&limit=&device_id=` | Changes after cursor `since`; `410` if the cursor expired (see below) |
| `GET /sync/snapshot?device_id=` | Every task you can see now, plus a cursor to pull from |
| `POST /sync/push` | Upload a batch of offline changes (see below) |
| `GET /users/{id}/devices` | That user's devices and when they last synced |

## Offline sync

Devices keep a local copy of the tasks they can see, work offline, and sync when
they reconnect: **push** their own changes first, then **pull** everything new.

### Pull

Every task change writes one `sync_feed` row for each user who can see the task:
the assignee and all of their seniors. Rows get a number (`seq`) from one global
counter. A device remembers the last `seq` it saw and asks for what came after:

```
GET /sync/pull?since=1042&limit=500&device_id=<uuid>
→ { "changes": [ { "seq": 1043, "kind": "upsert", "task_id": "…", "task": { … } },
                 { "seq": 1047, "kind": "revoked", "task_id": "…" } ],
    "next_cursor": 1047, "has_more": false }
```

| kind | Device should |
|---|---|
| `upsert` | Store the included task (current server copy) |
| `deleted` | Remove the task: it was deleted |
| `revoked` | Remove the task: you can no longer see it (reassigned away, or your team moved) |

Repeat with `since = next_cursor` while `has_more` is true. Feed writes take a
Postgres advisory lock so feed numbers always become visible in order, which means a
device never skips over an entry that commits late.

### Push

```
POST /sync/push
{ "device_id": "<uuid>", "device_name": "Field tablet 3",
  "changes": [
    { "op_id": "unique-per-device", "op": "upsert", "task_id": "<uuid>",
      "base_version": 0, "fields": { "title": "New task", "assigned_to": "<uuid>" } },
    { "op_id": "…", "op": "upsert", "task_id": "<uuid>", "base_version": 3,
      "fields": { "status": "done" } },
    { "op_id": "…", "op": "delete", "task_id": "<uuid>", "base_version": 4 } ] }
```

- `base_version: 0` with `upsert` creates the task with the device-generated `task_id`.
  Otherwise `base_version` is the version the device last saw, and `fields` holds only
  what changed.
- Each change gets its own result: `applied`, `conflict` (with the server's copy in
  `task`), `forbidden`, `not_found` or `invalid`. One bad change does not block the rest.
- Results are stored per `(device_id, op_id)`. Resending the same batch after a dropped
  connection returns the original results with `"duplicate": true` and changes nothing.
- The same permission rules as the REST API apply. A device ID belongs to the first user
  who uses it.

### First install and long-offline devices

A new device, or one whose cursor has expired, calls `GET /sync/snapshot`. It returns
every task the user can currently see plus a `cursor`. The device pushes any unsent
changes first, replaces its whole local copy with the snapshot, then pulls from
`cursor` as usual.

### Cleanup

Feed entries and saved push results are only bookkeeping, so a cleanup job deletes old
ones. It never touches tasks, users or devices.

- A feed entry is deleted once **every device of that user** has pulled past it, or once
  it is older than `FEED_RETENTION` (30 days), so one lost tablet cannot block cleanup.
- Saved push results are deleted after `OPS_RETENTION` (7 days).
- The highest deleted entry per user is saved in `sync_watermarks`. A pull with an older
  cursor gets **`410 Gone`** `{"error": "cursor_expired"}`, and the device downloads a
  snapshot instead.

The API runs cleanup at startup and then every `CLEANUP_INTERVAL`. Only one instance runs
it at a time (Postgres advisory lock). To run it from cron instead, set
`CLEANUP_INTERVAL=0` and schedule:

```bash
go run ./cmd/cleanup
```

## Tests

Integration tests hit a real Postgres and **drop all tables** in the database you point them at:

```bash
TEST_DATABASE_URL="host=localhost user=postgres password=postgres dbname=edgeaware_test sslmode=disable" go test ./...
```
