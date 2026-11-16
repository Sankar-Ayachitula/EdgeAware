// Package cleanup deletes sync bookkeeping that is no longer needed. It never
// touches tasks, users or devices.
//
//   - sync_feed rows are deleted once every device of that user has pulled past
//     them, or once they are older than FeedRetention (so one lost tablet cannot
//     block cleanup forever). The highest deleted seq per user is stored in
//     sync_watermarks; a device with an older cursor is told to download a snapshot.
//   - processed_ops rows (saved push results for safe retries) are deleted after
//     OpsRetention; retries only happen within minutes or hours.
package cleanup

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

type Options struct {
	FeedRetention time.Duration // default 30 days
	OpsRetention  time.Duration // default 7 days
	BatchSize     int           // rows per DELETE statement; default 10,000
}

func (o *Options) defaults() {
	if o.FeedRetention <= 0 {
		o.FeedRetention = 30 * 24 * time.Hour
	}
	if o.OpsRetention <= 0 {
		o.OpsRetention = 7 * 24 * time.Hour
	}
	if o.BatchSize <= 0 {
		o.BatchSize = 10_000
	}
}

type Result struct {
	FeedRowsDeleted int64
	OpsDeleted      int64
	Skipped         bool // another instance was already running cleanup
}

// Arbitrary constant; only one cleanup runs at a time across all API instances.
const lockKey = 7_311_202_612

// Run deletes old rows in batches. It is safe to run while the API is serving traffic.
func Run(ctx context.Context, db *gorm.DB, opts Options) (Result, error) {
	opts.defaults()
	var res Result

	// Session-level lock on one dedicated connection, so overlapping runs skip.
	conn, err := db.WithContext(ctx).DB()
	if err != nil {
		return res, err
	}
	c, err := conn.Conn(ctx)
	if err != nil {
		return res, err
	}
	defer c.Close()
	var got bool
	if err := c.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&got); err != nil {
		return res, err
	}
	if !got {
		res.Skipped = true
		return res, nil
	}
	defer c.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck

	feedCutoff := time.Now().Add(-opts.FeedRetention)
	for {
		// Delete one batch and raise each affected user's watermark in the same statement.
		var n int64
		err := db.WithContext(ctx).Raw(`
			WITH doomed AS (
				SELECT f.user_id, f.seq FROM sync_feed f
				WHERE f.created_at < ?
				   OR f.seq <= (SELECT MIN(d.last_pull_seq) FROM devices d WHERE d.user_id = f.user_id)
				LIMIT ?
			), del AS (
				DELETE FROM sync_feed f USING doomed
				WHERE f.user_id = doomed.user_id AND f.seq = doomed.seq
				RETURNING f.user_id, f.seq
			), marks AS (
				INSERT INTO sync_watermarks (user_id, pruned_through)
				SELECT user_id, MAX(seq) FROM del GROUP BY user_id
				ON CONFLICT (user_id) DO UPDATE
				SET pruned_through = GREATEST(sync_watermarks.pruned_through, EXCLUDED.pruned_through)
			)
			SELECT COUNT(*) FROM del`,
			feedCutoff, opts.BatchSize,
		).Scan(&n).Error
		if err != nil {
			return res, err
		}
		res.FeedRowsDeleted += n
		if n < int64(opts.BatchSize) {
			break
		}
	}

	opsCutoff := time.Now().Add(-opts.OpsRetention)
	for {
		r := db.WithContext(ctx).Exec(`
			DELETE FROM processed_ops WHERE ctid IN (
				SELECT ctid FROM processed_ops WHERE created_at < ? LIMIT ?
			)`, opsCutoff, opts.BatchSize)
		if r.Error != nil {
			return res, r.Error
		}
		res.OpsDeleted += r.RowsAffected
		if r.RowsAffected < int64(opts.BatchSize) {
			break
		}
	}
	return res, nil
}

// Every runs cleanup immediately and then at each interval until ctx is cancelled.
func Every(ctx context.Context, db *gorm.DB, interval time.Duration, opts Options) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		res, err := Run(ctx, db, opts)
		switch {
		case err != nil:
			log.Printf("cleanup failed: %v", err)
		case res.Skipped:
			log.Printf("cleanup skipped: another instance is running it")
		default:
			log.Printf("cleanup: deleted %d feed rows, %d saved push results", res.FeedRowsDeleted, res.OpsDeleted)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
