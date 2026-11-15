package server_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"

	"edgeaware/internal/models"
	syncpkg "edgeaware/internal/sync"
)

type pullResp struct {
	Changes    []syncpkg.Change `json:"changes"`
	NextCursor int64            `json:"next_cursor"`
	HasMore    bool             `json:"has_more"`
}

type pushResp struct {
	Results []syncpkg.OpResult `json:"results"`
}

// pullAll pulls from `since` until has_more is false, using small pages.
func (e *env) pullAll(token string, since int64) (map[string]syncpkg.Change, int64) {
	e.t.Helper()
	latest := map[string]syncpkg.Change{}
	for {
		var r pullResp
		expect(e.t, "pull", e.do(token, "GET", fmt.Sprintf("/sync/pull?since=%d&limit=2", since), nil, &r), 200)
		for _, c := range r.Changes {
			latest[c.TaskID] = c
		}
		since = r.NextCursor
		if !r.HasMore {
			return latest, since
		}
	}
}

func (e *env) createTask(token, title, assignee string) models.Task {
	e.t.Helper()
	var t models.Task
	expect(e.t, "create "+title, e.do(token, "POST", "/tasks", map[string]any{"title": title, "assigned_to": e.id(assignee)}, &t), 201)
	return t
}

func TestPullFollowsHierarchy(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	tasks := []models.Task{}
	for i := range 5 {
		tasks = append(tasks, e.createTask(sup, fmt.Sprintf("task %d", i), "operator_n1a"))
	}

	// Everyone from the operator up to the director receives all 5 (paged 2 at a time).
	for _, who := range []string{"operator_n1a", "supervisor_n1", "manager_north", "director"} {
		got, cursor := e.pullAll(e.login(who), 0)
		if len(got) != 5 {
			t.Errorf("%s pulled %d tasks, want 5", who, len(got))
		}
		for _, c := range got {
			if c.Kind != models.FeedUpsert || c.Task == nil {
				t.Errorf("%s: change %+v", who, c)
			}
		}
		// Pulling again from the cursor returns nothing new.
		more, _ := e.pullAll(e.login(who), cursor)
		if len(more) != 0 {
			t.Errorf("%s: %d changes after cursor", who, len(more))
		}
	}
	// Other branches and peers get nothing.
	for _, who := range []string{"manager_south", "operator_n1b", "supervisor_n2"} {
		if got, _ := e.pullAll(e.login(who), 0); len(got) != 0 {
			t.Errorf("%s pulled %d tasks, want 0", who, len(got))
		}
	}

	// An edit shows up for the operator as the new version.
	op := e.login("operator_n1a")
	_, cursor := e.pullAll(op, 0)
	expect(t, "edit", e.do(sup, "PATCH", "/tasks/"+tasks[0].ID, map[string]any{"version": 1, "title": "edited"}, nil), 200)
	got, _ := e.pullAll(op, cursor)
	if c := got[tasks[0].ID]; len(got) != 1 || c.Task == nil || c.Task.Title != "edited" || c.Task.Version != 2 {
		t.Errorf("after edit: %+v", got)
	}
}

func TestPullReassignDeleteAndMove(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	opA := e.login("operator_n1a")
	opB := e.login("operator_n1b")
	north := e.login("manager_north")
	south := e.login("manager_south")

	task := e.createTask(sup, "valve", "operator_n1a")
	_, curA := e.pullAll(opA, 0)
	_, curB := e.pullAll(opB, 0)

	// Reassign A -> B: A is told it lost access, B receives the task.
	expect(t, "reassign", e.do(sup, "PATCH", "/tasks/"+task.ID, map[string]any{"version": 1, "assigned_to": e.id("operator_n1b")}, nil), 200)
	gotA, curA := e.pullAll(opA, curA)
	if c := gotA[task.ID]; c.Kind != models.FeedRevoked || c.Task != nil {
		t.Errorf("A after reassign: %+v", c)
	}
	gotB, curB := e.pullAll(opB, curB)
	if c := gotB[task.ID]; c.Kind != models.FeedUpsert || c.Task == nil {
		t.Errorf("B after reassign: %+v", c)
	}
	// A from scratch: the old upsert is shown as revoked because A can no longer see it.
	if c := first(e.pullAll(opA, 0))[task.ID]; c.Kind != models.FeedRevoked || c.Task != nil {
		t.Errorf("A full pull: %+v", c)
	}

	// Move supervisor_n1's team to the south: north loses the task, south gains it.
	_, curN := e.pullAll(north, 0)
	_, curS := e.pullAll(south, 0)
	expect(t, "move", e.do(e.login("director"), "PATCH", "/users/"+e.id("supervisor_n1")+"/manager", map[string]any{"manager_id": e.id("manager_south")}, nil), 204)
	if c := first(e.pullAll(north, curN))[task.ID]; c.Kind != models.FeedRevoked {
		t.Errorf("north after move: %+v", c)
	}
	gotS, _ := e.pullAll(south, curS)
	if c := gotS[task.ID]; c.Kind != models.FeedUpsert || c.Task == nil {
		t.Errorf("south after move: %+v", c)
	}
	// The supervisor did not move relative to its own team, so it gets nothing new.
	_, curSup := e.pullAll(sup, 0)

	// Delete: everyone who could see it gets "deleted".
	expect(t, "delete", e.do(sup, "DELETE", "/tasks/"+task.ID, nil, nil), 204)
	if c := first(e.pullAll(opB, curB))[task.ID]; c.Kind != models.FeedDeleted {
		t.Errorf("B after delete: %+v", c)
	}
	if c := first(e.pullAll(sup, curSup))[task.ID]; c.Kind != models.FeedDeleted {
		t.Errorf("supervisor after delete: %+v", c)
	}
}

func TestPushOfflineWork(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	op := e.login("operator_n1a")
	supDevice := uuid.NewString()
	opDevice := uuid.NewString()
	taskID := uuid.NewString()

	// The supervisor creates a task offline and pushes it.
	var r pushResp
	createBatch := map[string]any{
		"device_id":   supDevice,
		"device_name": "supervisor tablet",
		"changes": []map[string]any{{
			"op_id": "op-1", "op": "upsert", "task_id": taskID, "base_version": 0,
			"fields": map[string]any{"title": "Offline task", "assigned_to": e.id("operator_n1a")},
		}},
	}
	expect(t, "push create", e.do(sup, "POST", "/sync/push", createBatch, &r), 200)
	if len(r.Results) != 1 || r.Results[0].Status != syncpkg.StatusApplied || r.Results[0].Task.ID != taskID {
		t.Fatalf("create result: %+v", r.Results)
	}

	// The network dropped the response; the device retries the same batch.
	expect(t, "push retry", e.do(sup, "POST", "/sync/push", createBatch, &r), 200)
	if res := r.Results[0]; !res.Duplicate || res.Status != syncpkg.StatusApplied {
		t.Errorf("retry result: %+v", res)
	}
	var list []models.Task
	e.do(sup, "GET", "/users/"+e.id("operator_n1a")+"/tasks", nil, &list)
	if len(list) != 1 {
		t.Errorf("tasks after retry = %d, want 1", len(list))
	}

	// Operator (offline) starts it; supervisor (offline) renames it. Both based on v1.
	expect(t, "op push", e.do(op, "POST", "/sync/push", map[string]any{
		"device_id": opDevice,
		"changes": []map[string]any{{
			"op_id": "a", "op": "upsert", "task_id": taskID, "base_version": 1, "fields": map[string]any{"status": "in_progress"},
		}},
	}, &r), 200)
	if r.Results[0].Status != syncpkg.StatusApplied || r.Results[0].Task.Version != 2 {
		t.Errorf("operator push: %+v", r.Results[0])
	}
	expect(t, "sup push stale", e.do(sup, "POST", "/sync/push", map[string]any{
		"device_id": supDevice,
		"changes": []map[string]any{{
			"op_id": "op-2", "op": "upsert", "task_id": taskID, "base_version": 1, "fields": map[string]any{"title": "Renamed"},
		}},
	}, &r), 200)
	if res := r.Results[0]; res.Status != syncpkg.StatusConflict || res.Task == nil || res.Task.Version != 2 || res.Task.Status != models.StatusInProgress {
		t.Errorf("stale push: %+v", res)
	}

	// One batch with mixed outcomes: each change gets its own result.
	expect(t, "mixed batch", e.do(op, "POST", "/sync/push", map[string]any{
		"device_id": opDevice,
		"changes": []map[string]any{
			{"op_id": "b", "op": "upsert", "task_id": taskID, "base_version": 2, "fields": map[string]any{"title": "operator rename"}},
			{"op_id": "c", "op": "upsert", "task_id": taskID, "base_version": 2, "fields": map[string]any{"status": "done"}},
			{"op_id": "d", "op": "upsert", "task_id": uuid.NewString(), "base_version": 0, "fields": map[string]any{"title": "operator task"}},
			{"op_id": "e", "op": "delete", "task_id": uuid.NewString(), "base_version": 1},
			{"op_id": "f", "op": "explode", "task_id": taskID},
			{"op_id": "g", "op": "upsert", "task_id": taskID, "base_version": 3, "fields": map[string]any{"status": "bogus"}},
		},
	}, &r), 200)
	want := []string{
		syncpkg.StatusForbidden, // operators cannot rename
		syncpkg.StatusApplied,   // status change is allowed
		syncpkg.StatusForbidden, // operators cannot create tasks
		syncpkg.StatusNotFound,  // unknown task
		syncpkg.StatusInvalid,   // unknown op
		syncpkg.StatusInvalid,   // unknown status
	}
	for i, w := range want {
		if i >= len(r.Results) || r.Results[i].Status != w {
			t.Errorf("mixed[%d] = %+v, want %s", i, r.Results, w)
			break
		}
	}

	// Delete with a stale version conflicts; with the current one it applies.
	expect(t, "delete stale", e.do(sup, "POST", "/sync/push", map[string]any{
		"device_id": supDevice,
		"changes":   []map[string]any{{"op_id": "op-3", "op": "delete", "task_id": taskID, "base_version": 1}},
	}, &r), 200)
	if r.Results[0].Status != syncpkg.StatusConflict {
		t.Errorf("stale delete: %+v", r.Results[0])
	}
	expect(t, "delete", e.do(sup, "POST", "/sync/push", map[string]any{
		"device_id": supDevice,
		"changes":   []map[string]any{{"op_id": "op-4", "op": "delete", "task_id": taskID, "base_version": 3}},
	}, &r), 200)
	if r.Results[0].Status != syncpkg.StatusApplied {
		t.Errorf("delete: %+v", r.Results[0])
	}
	// The ID of a deleted task cannot be reused.
	expect(t, "reuse id", e.do(sup, "POST", "/sync/push", map[string]any{
		"device_id": supDevice,
		"changes": []map[string]any{{
			"op_id": "op-5", "op": "upsert", "task_id": taskID, "base_version": 0,
			"fields": map[string]any{"title": "again", "assigned_to": e.id("operator_n1a")},
		}},
	}, &r), 200)
	if r.Results[0].Status != syncpkg.StatusConflict {
		t.Errorf("reused id: %+v", r.Results[0])
	}

	// Pull on the operator's device sees the deletion, and records its cursor.
	var p pullResp
	expect(t, "pull with device", e.do(op, "GET", "/sync/pull?device_id="+opDevice, nil, &p), 200)
	if c := p.Changes[len(p.Changes)-1]; c.TaskID != taskID || c.Kind != models.FeedDeleted {
		t.Errorf("last change: %+v", c)
	}
	var devices []models.Device
	expect(t, "supervisor sees operator devices", e.do(sup, "GET", "/users/"+e.id("operator_n1a")+"/devices", nil, &devices), 200)
	if len(devices) != 1 || devices[0].ID != opDevice || devices[0].LastPullSeq != p.NextCursor {
		t.Errorf("devices = %+v (cursor %d)", devices, p.NextCursor)
	}
	expect(t, "operator cannot see supervisor devices", e.do(op, "GET", "/users/"+e.id("supervisor_n1")+"/devices", nil, nil), 403)

	// A device ID belongs to its first user.
	expect(t, "stolen device id", e.do(op, "POST", "/sync/push", map[string]any{
		"device_id": supDevice,
		"changes":   []map[string]any{{"op_id": "x", "op": "delete", "task_id": taskID, "base_version": 1}},
	}, nil), 403)
	expect(t, "missing device id", e.do(op, "POST", "/sync/push", map[string]any{
		"changes": []map[string]any{{"op_id": "x", "op": "delete", "task_id": taskID, "base_version": 1}},
	}, nil), 400)
}

func first[A, B any](a A, _ B) A { return a }

// Concurrent writers must not deadlock on the feed lock or lose feed entries, and
// a device pulling while writes are in flight must still end up with every task.
func TestConcurrentWritesAndPulls(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	north := e.login("manager_north")
	op := e.login("operator_n1a")

	const n = 40
	done := make(chan struct{})
	errs := make(chan error, n)
	go func() {
		defer close(done)
		sem := make(chan struct{}, 8)
		finished := make(chan struct{}, n)
		for i := range n {
			sem <- struct{}{}
			go func(i int) {
				defer func() { <-sem; finished <- struct{}{} }()
				token, who := sup, "operator_n1a"
				if i%2 == 1 {
					token, who = north, "operator_n2a"
				}
				code := e.do(token, "POST", "/tasks", map[string]any{"title": fmt.Sprint("t", i), "assigned_to": e.id(who)}, nil)
				if code != 201 {
					errs <- fmt.Errorf("create %d: status %d", i, code)
				}
			}(i)
		}
		for range n {
			<-finished
		}
	}()

	// Pull repeatedly while the writes run, the way a device would.
	seen := map[string]bool{}
	var cursor int64
	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
		}
		got, next := e.pullAll(op, cursor)
		for id := range got {
			seen[id] = true
		}
		cursor = next
	}
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	got, _ := e.pullAll(op, cursor)
	for id := range got {
		seen[id] = true
	}
	if len(seen) != n/2 {
		t.Errorf("operator saw %d tasks, want %d", len(seen), n/2)
	}
	if all, _ := e.pullAll(e.login("director"), 0); len(all) != n {
		t.Errorf("director saw %d tasks, want %d", len(all), n)
	}
}
