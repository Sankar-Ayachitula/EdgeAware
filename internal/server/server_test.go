package server_test

// Integration tests over HTTP against a real Postgres database.
// They run only when TEST_DATABASE_URL is set, and they DROP all tables in it:
//
//	TEST_DATABASE_URL="host=localhost user=postgres password=postgres dbname=edgeaware_test sslmode=disable" go test ./...

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"edgeaware/internal/config"
	"edgeaware/internal/db"
	"edgeaware/internal/models"
	"edgeaware/internal/seed"
	"edgeaware/internal/server"
)

type env struct {
	t     *testing.T
	db    *gorm.DB
	srv   *httptest.Server
	users map[string]models.User
	roles map[string]uint
}

func setup(t *testing.T) *env {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	gdb, err := db.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	users, err := seed.Run(gdb)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]uint{}
	for _, u := range users {
		roles[u.Role.Name] = u.RoleID
	}
	cfg := config.Config{JWTSecret: []byte("test-secret"), TokenTTL: time.Hour}
	srv := httptest.NewServer(server.New(gdb, cfg))
	t.Cleanup(srv.Close)
	return &env{t: t, db: gdb, srv: srv, users: users, roles: roles}
}

func (e *env) id(username string) string { return e.users[username].ID }

func (e *env) login(username string) string {
	e.t.Helper()
	var out struct {
		AccessToken string `json:"access_token"`
	}
	code := e.do("", "POST", "/login", map[string]string{"username": username, "password": seed.DevPassword}, &out)
	if code != http.StatusOK {
		e.t.Fatalf("login %s: status %d", username, code)
	}
	return out.AccessToken
}

// do sends a request as the given token and decodes the JSON response into out (if non-nil).
func (e *env) do(token, method, path string, body, out any) int {
	e.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func expect(t *testing.T, what string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: status %d, want %d", what, got, want)
	}
}

func names(users []models.User) []string {
	out := make([]string, len(users))
	for i, u := range users {
		out[i] = u.Username
	}
	return out
}

func TestLoginAndAuth(t *testing.T) {
	e := setup(t)
	expect(t, "wrong password", e.do("", "POST", "/login", map[string]string{"username": "director", "password": "nope"}, nil), 401)
	expect(t, "no token", e.do("", "GET", "/me", nil, nil), 401)
	expect(t, "bad token", e.do("garbage", "GET", "/me", nil, nil), 401)

	var me models.User
	expect(t, "me", e.do(e.login("supervisor_n1"), "GET", "/me", nil, &me), 200)
	if me.Username != "supervisor_n1" || me.Role.Level != 1 {
		t.Errorf("me = %s level %d", me.Username, me.Role.Level)
	}
}

func TestHierarchyVisibility(t *testing.T) {
	e := setup(t)
	dir := e.login("director")
	north := e.login("manager_north")
	op := e.login("operator_n1a")

	var juniors []models.User
	expect(t, "director juniors", e.do(dir, "GET", "/users/"+e.id("director")+"/juniors", nil, &juniors), 200)
	if got := fmt.Sprint(names(juniors)); got != "[manager_north manager_south]" {
		t.Errorf("director juniors = %s", got)
	}

	// Drilling down: the director opens manager_north's tab, then supervisor_n1's.
	expect(t, "director -> north juniors", e.do(dir, "GET", "/users/"+e.id("manager_north")+"/juniors", nil, &juniors), 200)
	if len(juniors) != 2 {
		t.Errorf("manager_north juniors = %v", names(juniors))
	}
	var team []models.User
	expect(t, "north team", e.do(north, "GET", "/users/"+e.id("manager_north")+"/team", nil, &team), 200)
	if len(team) != 5 { // 2 supervisors + 3 operators
		t.Errorf("manager_north team = %v", names(team))
	}

	// Lower levels cannot see upper levels; nobody can see another branch.
	expect(t, "operator sees supervisor", e.do(op, "GET", "/users/"+e.id("supervisor_n1"), nil, nil), 403)
	expect(t, "operator sees director", e.do(op, "GET", "/users/"+e.id("director"), nil, nil), 403)
	expect(t, "north sees south manager", e.do(north, "GET", "/users/"+e.id("manager_south"), nil, nil), 403)
	expect(t, "north sees south operator", e.do(north, "GET", "/users/"+e.id("operator_s1a"), nil, nil), 403)
	expect(t, "north sees own operator", e.do(north, "GET", "/users/"+e.id("operator_n2a"), nil, nil), 200)
	expect(t, "bad id", e.do(north, "GET", "/users/not-a-uuid", nil, nil), 404)
}

func TestTaskPermissions(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	north := e.login("manager_north")
	south := e.login("manager_south")
	op := e.login("operator_n1a")
	opOther := e.login("operator_n1b")

	// A supervisor assigns a task to their operator.
	var task models.Task
	expect(t, "create", e.do(sup, "POST", "/tasks", map[string]any{
		"title": "Inspect valve 7", "assigned_to": e.id("operator_n1a"),
	}, &task), 201)
	if task.Version != 1 || task.Status != models.StatusTodo || task.CreatedBy != e.id("supervisor_n1") {
		t.Fatalf("created task = %+v", task)
	}

	// Operators cannot create tasks, or assign upward / across branches.
	expect(t, "operator creates for self", e.do(op, "POST", "/tasks", map[string]any{"title": "x"}, nil), 403)
	expect(t, "supervisor assigns to boss", e.do(sup, "POST", "/tasks", map[string]any{"title": "x", "assigned_to": e.id("manager_north")}, nil), 403)
	expect(t, "south assigns to north op", e.do(south, "POST", "/tasks", map[string]any{"title": "x", "assigned_to": e.id("operator_n1a")}, nil), 403)

	// Visibility of the task follows the hierarchy.
	expect(t, "assignee sees", e.do(op, "GET", "/tasks/"+task.ID, nil, nil), 200)
	expect(t, "manager two levels up sees", e.do(north, "GET", "/tasks/"+task.ID, nil, nil), 200)
	expect(t, "other branch hidden", e.do(south, "GET", "/tasks/"+task.ID, nil, nil), 403)
	expect(t, "peer operator hidden", e.do(opOther, "GET", "/tasks/"+task.ID, nil, nil), 403)
	var list []models.Task
	expect(t, "north lists op tasks", e.do(north, "GET", "/users/"+e.id("operator_n1a")+"/tasks", nil, &list), 200)
	if len(list) != 1 {
		t.Errorf("listed %d tasks", len(list))
	}

	// The assignee may change status only.
	expect(t, "assignee edits title", e.do(op, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 1, "title": "hacked"}, nil), 403)
	expect(t, "assignee starts", e.do(op, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 1, "status": "in_progress"}, &task), 200)
	if task.Version != 2 {
		t.Errorf("version after update = %d", task.Version)
	}

	// Stale version -> 409 with the server copy.
	var conflict struct {
		Current models.Task `json:"current"`
	}
	expect(t, "stale version", e.do(north, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 1, "title": "Renamed"}, &conflict), 409)
	if conflict.Current.Version != 2 {
		t.Errorf("conflict current version = %d", conflict.Current.Version)
	}

	// A senior edits fields; done sets completed_at; clearing due_at with null works.
	due := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	expect(t, "manager edits", e.do(north, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 2, "title": "Renamed", "due_at": due}, &task), 200)
	if task.Title != "Renamed" || task.DueAt == nil || !task.DueAt.Equal(due) {
		t.Errorf("after manager edit: %+v", task)
	}
	expect(t, "done", e.do(op, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 3, "status": "done"}, &task), 200)
	if task.CompletedAt == nil {
		t.Error("completed_at not set")
	}
	expect(t, "clear due", e.do(sup, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 4, "due_at": nil}, &task), 200)
	if task.DueAt != nil {
		t.Error("due_at not cleared")
	}

	// Reassign within the supervisor's team; not outside it.
	expect(t, "reassign outside", e.do(sup, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 5, "assigned_to": e.id("operator_n2a")}, nil), 403)
	expect(t, "reassign inside", e.do(sup, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 5, "assigned_to": e.id("operator_n1b")}, &task), 200)
	expect(t, "old assignee loses access", e.do(op, "GET", "/tasks/"+task.ID, nil, nil), 403)

	// Delete: assignee cannot, supervisor can.
	expect(t, "assignee deletes", e.do(opOther, "DELETE", "/tasks/"+task.ID, nil, nil), 403)
	expect(t, "supervisor deletes", e.do(sup, "DELETE", "/tasks/"+task.ID, nil, nil), 204)
	expect(t, "gone", e.do(sup, "GET", "/tasks/"+task.ID, nil, nil), 404)

	// Offline-created tasks keep their client-generated ID; reusing it conflicts.
	clientID := uuid.NewString()
	expect(t, "client id", e.do(sup, "POST", "/tasks", map[string]any{"id": clientID, "title": "offline", "assigned_to": e.id("operator_n1a")}, &task), 201)
	if task.ID != clientID {
		t.Errorf("id = %s, want %s", task.ID, clientID)
	}
	expect(t, "duplicate id", e.do(sup, "POST", "/tasks", map[string]any{"id": clientID, "title": "again", "assigned_to": e.id("operator_n1a")}, nil), 409)
}

func TestCreateAndMoveUsers(t *testing.T) {
	e := setup(t)
	north := e.login("manager_north")
	sup := e.login("supervisor_n1")

	// A manager onboards an operator under one of their supervisors.
	var newOp models.User
	expect(t, "create operator", e.do(north, "POST", "/users", map[string]any{
		"username": "operator_new", "password": seed.DevPassword, "role_id": e.roles["Field Operator"], "manager_id": e.id("supervisor_n2"),
	}, &newOp), 201)
	var team []models.User
	e.do(north, "GET", "/users/"+e.id("manager_north")+"/team", nil, &team)
	if len(team) != 6 {
		t.Errorf("team after create = %v", names(team))
	}
	expect(t, "new user can log in", e.do(e.login("operator_new"), "GET", "/me", nil, nil), 200)

	// Role level must be below the manager's; cannot add users in another branch.
	expect(t, "same level as manager", e.do(north, "POST", "/users", map[string]any{
		"username": "x1", "password": "longenough", "role_id": e.roles["Supervisor"], "manager_id": e.id("supervisor_n1"),
	}, nil), 400)
	expect(t, "other branch", e.do(north, "POST", "/users", map[string]any{
		"username": "x2", "password": "longenough", "role_id": e.roles["Field Operator"], "manager_id": e.id("supervisor_s1"),
	}, nil), 403)
	expect(t, "duplicate username", e.do(north, "POST", "/users", map[string]any{
		"username": "operator_new", "password": "longenough", "role_id": e.roles["Field Operator"], "manager_id": e.id("supervisor_n2"),
	}, nil), 409)

	// Move operator_n1a from supervisor_n1 to supervisor_n2.
	expect(t, "supervisor cannot move", e.do(sup, "PATCH", "/users/"+e.id("operator_n1a")+"/manager", map[string]any{"manager_id": e.id("supervisor_n2")}, nil), 403)
	expect(t, "manager moves op", e.do(north, "PATCH", "/users/"+e.id("operator_n1a")+"/manager", map[string]any{"manager_id": e.id("supervisor_n2")}, nil), 204)
	expect(t, "old supervisor loses access", e.do(sup, "GET", "/users/"+e.id("operator_n1a"), nil, nil), 403)
	expect(t, "new supervisor gains access", e.do(e.login("supervisor_n2"), "GET", "/users/"+e.id("operator_n1a"), nil, nil), 200)

	// Move a whole subtree: supervisor_n1 (with its remaining operator) to the south.
	dir := e.login("director")
	expect(t, "move subtree", e.do(dir, "PATCH", "/users/"+e.id("supervisor_n1")+"/manager", map[string]any{"manager_id": e.id("manager_south")}, nil), 204)
	south := e.login("manager_south")
	expect(t, "south sees moved op", e.do(south, "GET", "/users/"+e.id("operator_n1b"), nil, nil), 200)
	expect(t, "north lost moved op", e.do(north, "GET", "/users/"+e.id("operator_n1b"), nil, nil), 403)
	expect(t, "director still sees", e.do(dir, "GET", "/users/"+e.id("operator_n1b"), nil, nil), 200)

	// Loops and level violations are rejected.
	expect(t, "loop", e.do(dir, "PATCH", "/users/"+e.id("manager_south")+"/manager", map[string]any{"manager_id": e.id("supervisor_s1")}, nil), 400)
	expect(t, "lower-level manager", e.do(dir, "PATCH", "/users/"+e.id("supervisor_s1")+"/manager", map[string]any{"manager_id": e.id("operator_s1a")}, nil), 400)
}
