package server_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"edgeaware/internal/cleanup"
	"edgeaware/internal/models"
	syncpkg "edgeaware/internal/sync"
)

func (e *env) feedCount(username string) int64 {
	e.t.Helper()
	var n int64
	if err := e.db.Model(&models.SyncFeed{}).Where("user_id = ?", e.id(username)).Count(&n).Error; err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) runCleanup(opts cleanup.Options) cleanup.Result {
	e.t.Helper()
	res, err := cleanup.Run(context.Background(), e.db, opts)
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

func (e *env) taskCount() int64 {
	var n int64
	e.db.Unscoped().Model(&models.Task{}).Count(&n)
	return n
}

type snapshotResp struct {
	Tasks  []models.Task `json:"tasks"`
	Cursor int64         `json:"cursor"`
}

// Rows every device has pulled are deleted; rows not yet pulled are kept.
// Tasks are never touched.
func TestCleanupAfterDevicesPulled(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	op := e.login("operator_n1a")
	phone, tablet := uuid.NewString(), uuid.NewString()

	for i := range 3 {
		e.createTask(sup, fmt.Sprint("first ", i), "operator_n1a")
	}
	// The phone pulls everything; the tablet pulls nothing yet.
	var p pullResp
	expect(t, "phone pull", e.do(op, "GET", "/sync/pull?device_id="+phone, nil, &p), 200)
	expect(t, "tablet registers", e.do(op, "GET", "/sync/pull?limit=1&device_id="+tablet, nil, nil), 200)
	phoneCursor := p.NextCursor

	tasksBefore := e.taskCount()
	e.runCleanup(cleanup.Options{})
	// The tablet has only pulled the first entry, so only that one may go.
	if n := e.feedCount("operator_n1a"); n != 2 {
		t.Errorf("operator feed rows after first cleanup = %d, want 2", n)
	}
	// The tablet catches up; now all 3 can go.
	expect(t, "tablet pull", e.do(op, "GET", "/sync/pull?since=0&device_id="+tablet, nil, &p), 410)
	expect(t, "tablet snapshot", e.do(op, "GET", "/sync/snapshot?device_id="+tablet, nil, nil), 200)
	e.runCleanup(cleanup.Options{})
	if n := e.feedCount("operator_n1a"); n != 0 {
		t.Errorf("operator feed rows after catch-up = %d, want 0", n)
	}
	if e.taskCount() != tasksBefore {
		t.Errorf("cleanup changed the tasks table")
	}

	// The phone keeps syncing normally from its cursor.
	e.createTask(sup, "after cleanup", "operator_n1a")
	expect(t, "phone pull again", e.do(op, "GET", fmt.Sprintf("/sync/pull?since=%d&device_id=%s", phoneCursor, phone), nil, &p), 200)
	if len(p.Changes) != 1 || p.Changes[0].Task == nil || p.Changes[0].Task.Title != "after cleanup" {
		t.Errorf("phone after cleanup: %+v", p.Changes)
	}

	// Users with no registered devices keep their rows until the time limit.
	if n := e.feedCount("director"); n != 4 {
		t.Errorf("director feed rows = %d, want 4 (no devices, nothing old)", n)
	}
}

// A device offline past the retention window gets 410, downloads a snapshot,
// and continues from the snapshot's cursor without missing anything.
func TestCleanupExpiredCursorAndSnapshot(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	op := e.login("operator_n1a")
	tablet := uuid.NewString()

	kept := e.createTask(sup, "kept", "operator_n1a")
	gone := e.createTask(sup, "deleted later", "operator_n1a")
	expect(t, "tablet first pull", e.do(op, "GET", "/sync/pull?device_id="+tablet, nil, nil), 200)
	expect(t, "reassign away", e.do(sup, "PATCH", "/tasks/"+gone.ID, map[string]any{"version": 1, "assigned_to": e.id("operator_n1b")}, nil), 200)
	e.createTask(sup, "new while offline", "operator_n1a")
	var p pullResp
	expect(t, "tablet pull before cleanup", e.do(op, "GET", "/sync/pull?since=0", nil, &p), 200)

	// The tablet goes in a drawer. Everything ages past the retention window.
	if err := e.db.Exec(`UPDATE sync_feed SET created_at = now() - interval '40 days'`).Error; err != nil {
		t.Fatal(err)
	}
	res := e.runCleanup(cleanup.Options{FeedRetention: 30 * 24 * time.Hour})
	if res.FeedRowsDeleted == 0 {
		t.Fatal("nothing deleted")
	}

	// Its old cursor is now behind the watermark.
	var gonePayload map[string]string
	expect(t, "stale cursor", e.do(op, "GET", "/sync/pull?since=2&device_id="+tablet, nil, &gonePayload), 410)
	if gonePayload["error"] != "cursor_expired" {
		t.Errorf("410 body = %v", gonePayload)
	}

	// The snapshot has exactly the tasks the operator can see now.
	var snap snapshotResp
	expect(t, "snapshot", e.do(op, "GET", "/sync/snapshot?device_id="+tablet, nil, &snap), 200)
	titles := map[string]bool{}
	for _, task := range snap.Tasks {
		titles[task.Title] = true
	}
	if len(snap.Tasks) != 2 || !titles["kept"] || !titles["new while offline"] {
		t.Errorf("snapshot tasks = %v", titles)
	}

	// Pulling from the snapshot cursor works and picks up later changes.
	expect(t, "edit after snapshot", e.do(sup, "PATCH", "/tasks/"+kept.ID, map[string]any{"version": 1, "title": "kept v2"}, nil), 200)
	expect(t, "pull from snapshot cursor", e.do(op, "GET", fmt.Sprintf("/sync/pull?since=%d&device_id=%s", snap.Cursor, tablet), nil, &p), 200)
	if len(p.Changes) != 1 || p.Changes[0].Task.Title != "kept v2" {
		t.Errorf("after snapshot: %+v", p.Changes)
	}

	// The device's stored cursor followed along.
	var d models.Device
	e.db.First(&d, "id = ?", tablet)
	if d.LastPullSeq != p.NextCursor {
		t.Errorf("device cursor = %d, want %d", d.LastPullSeq, p.NextCursor)
	}

	// A brand-new device must also start from a snapshot once rows were cleaned up.
	expect(t, "new device from zero", e.do(op, "GET", "/sync/pull?since=0", nil, nil), 410)
	// Other users who were never pruned can still pull from zero.
	expect(t, "unaffected user", e.do(e.login("manager_south"), "GET", "/sync/pull?since=0", nil, nil), 200)
}

// Saved push results expire; recent ones still protect retries.
func TestCleanupSavedPushResults(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	device := uuid.NewString()
	push := func(opID, title string) syncpkg.OpResult {
		var r pushResp
		expect(t, "push "+opID, e.do(sup, "POST", "/sync/push", map[string]any{
			"device_id": device,
			"changes": []map[string]any{{
				"op_id": opID, "op": "upsert", "task_id": uuid.NewString(), "base_version": 0,
				"fields": map[string]any{"title": title, "assigned_to": e.id("operator_n1a")},
			}},
		}, &r), 200)
		return r.Results[0]
	}
	push("old", "old op")
	push("recent", "recent op")
	if err := e.db.Exec(`UPDATE processed_ops SET created_at = now() - interval '8 days' WHERE op_id = 'old'`).Error; err != nil {
		t.Fatal(err)
	}

	res := e.runCleanup(cleanup.Options{OpsRetention: 7 * 24 * time.Hour})
	if res.OpsDeleted != 1 {
		t.Errorf("ops deleted = %d, want 1", res.OpsDeleted)
	}
	var left []string
	e.db.Model(&models.ProcessedOp{}).Pluck("op_id", &left)
	if len(left) != 1 || left[0] != "recent" {
		t.Errorf("remaining ops = %v", left)
	}
}

// Small batches still delete everything, and overlapping runs skip instead of clashing.
func TestCleanupBatchesAndLock(t *testing.T) {
	e := setup(t)
	sup := e.login("supervisor_n1")
	for i := range 7 {
		e.createTask(sup, fmt.Sprint("t", i), "operator_n1a")
	}
	e.db.Exec(`UPDATE sync_feed SET created_at = now() - interval '40 days'`)
	res := e.runCleanup(cleanup.Options{BatchSize: 3})
	if res.FeedRowsDeleted != 7*4 { // operator, supervisor, manager, director
		t.Errorf("deleted %d rows, want 28", res.FeedRowsDeleted)
	}

	// Hold the cleanup lock from another connection: the run must skip.
	conn, _ := e.db.DB()
	c, err := conn.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.ExecContext(context.Background(), `SELECT pg_advisory_lock(7311202612)`); err != nil {
		t.Fatal(err)
	}
	if res := e.runCleanup(cleanup.Options{}); !res.Skipped {
		t.Error("cleanup ran while another holder had the lock")
	}
	c.ExecContext(context.Background(), `SELECT pg_advisory_unlock(7311202612)`)
}
