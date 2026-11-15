# EdgeAware Backend

Go backend for managers and field operators: a multi-level hierarchy of users,
tasks assigned down that hierarchy, and (next) offline sync for field devices.

Stack: Go standard-library router, GORM on PostgreSQL, JWT auth, bcrypt passwords.

## Run locally

```bash
docker compose up -d          # Postgres on :5432 (skip if you already run Postgres)
go run ./cmd/seed             # creates tables + a sample 4-level org (empty DB only)
JWT_SECRET=change-me go run ./cmd/api
```

The seeded users and their shared development password are in
[internal/seed/seed.go](internal/seed/seed.go).

Environment: `DATABASE_URL` (or `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`),
`JWT_SECRET`, `TOKEN_TTL` (default `15m`), `PORT` (default `8080`).

## How the hierarchy works

| Table | Purpose |
|---|---|
| `roles` | Name + numeric `level`. 0 = field operator; higher = more senior. |
| `users` | Each user has a `role_id` and a `manager_id` (their direct boss). |
| `user_hierarchy` | Closure table: one row for every (senior, junior) pair at every depth. |
| `tasks` | `assigned_to`, `created_by`, status, due date, `completed_at`, `version`. |

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
| `DELETE /tasks/{id}` | Soft delete |

## Tests

Integration tests hit a real Postgres and **drop all tables** in the database you point them at:

```bash
TEST_DATABASE_URL="host=localhost user=postgres password=postgres dbname=edgeaware_test sslmode=disable" go test ./...
```
