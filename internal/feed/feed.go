// Package feed writes the per-user sync feed. Every function must be called inside
// the same transaction as the change it records.
//
// Ordering guarantee: a device remembers the highest seq it has pulled and later asks
// for seq > cursor. That is only safe if seqs become visible in increasing order. Two
// transactions could otherwise take seqs 10 and 11 and commit 11 first; a device
// pulling in between would jump past 10 forever. To prevent that, feed writes take a
// transaction-level advisory lock before allocating seqs, so feed-writing transactions
// commit in seq order. This serializes task writes at the very end of each
// transaction, which is fine at this scale.
package feed

import (
	"gorm.io/gorm"

	"edgeaware/internal/models"
)

// Arbitrary constant identifying the feed lock.
const lockKey = 7_311_202_611

func lock(tx *gorm.DB) error {
	return tx.Exec(`SELECT pg_advisory_xact_lock(?)`, lockKey).Error
}

// Audience returns everyone who can see tasks assigned to assigneeID:
// the assignee and all of their seniors.
func Audience(tx *gorm.DB, assigneeID string) ([]string, error) {
	var ids []string
	err := tx.Model(&models.UserHierarchy{}).
		Where("descendant_id = ?", assigneeID).
		Pluck("ancestor_id", &ids).Error
	return ids, err
}

// TaskChanged records a create or update. If the task moved from oldAssignee to
// newAssignee (oldAssignee "" for a new task), people who could see it before but
// not now get a revoked entry.
func TaskChanged(tx *gorm.DB, taskID, oldAssignee, newAssignee string) error {
	if err := lock(tx); err != nil {
		return err
	}
	newAud, err := Audience(tx, newAssignee)
	if err != nil {
		return err
	}
	if err := insert(tx, newAud, taskID, models.FeedUpsert); err != nil {
		return err
	}
	if oldAssignee == "" || oldAssignee == newAssignee {
		return nil
	}
	oldAud, err := Audience(tx, oldAssignee)
	if err != nil {
		return err
	}
	return insert(tx, minus(oldAud, newAud), taskID, models.FeedRevoked)
}

// TaskDeleted tells everyone who could see the task that it is gone.
func TaskDeleted(tx *gorm.DB, taskID, assignee string) error {
	if err := lock(tx); err != nil {
		return err
	}
	aud, err := Audience(tx, assignee)
	if err != nil {
		return err
	}
	return insert(tx, aud, taskID, models.FeedDeleted)
}

// SubtreeMoved is called after userID (and their team) got a new manager. Seniors
// who lost the team get revoked entries for all of its live tasks; seniors who
// gained it get upserts.
func SubtreeMoved(tx *gorm.DB, userID string, oldSeniors, newSeniors []string) error {
	lost, gained := minus(oldSeniors, newSeniors), minus(newSeniors, oldSeniors)
	if len(lost) == 0 && len(gained) == 0 {
		return nil
	}
	if err := lock(tx); err != nil {
		return err
	}
	var taskIDs []string
	if err := tx.Model(&models.Task{}).
		Where("assigned_to IN (SELECT descendant_id FROM user_hierarchy WHERE ancestor_id = ?)", userID).
		Order("id").
		Pluck("id", &taskIDs).Error; err != nil {
		return err
	}
	for _, t := range taskIDs {
		if err := insert(tx, lost, t, models.FeedRevoked); err != nil {
			return err
		}
		if err := insert(tx, gained, t, models.FeedUpsert); err != nil {
			return err
		}
	}
	return nil
}

func insert(tx *gorm.DB, userIDs []string, taskID, kind string) error {
	for _, u := range userIDs {
		if err := tx.Exec(
			`INSERT INTO sync_feed (user_id, seq, task_id, kind) VALUES (?, nextval('sync_seq'), ?, ?)`,
			u, taskID, kind,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// minus returns the items of a that are not in b.
func minus(a, b []string) []string {
	in := make(map[string]bool, len(b))
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}
