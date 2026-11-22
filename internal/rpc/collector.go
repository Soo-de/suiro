package rpc

import (
	"context"
	"errors"

	"github.com/Soo-de/suiro/internal/model"
)

// ErrUnknownSource is returned for a source the collector is not watching.
var ErrUnknownSource = errors.New("rpc: source is not watched")

// ErrNoCompleteRead is returned by GetSnapshot for a key of a watched source
// that has no complete read: not yet, or no longer (a closed pull request).
// It is an answer, not a failure of the call: the server waits for the key
// to appear in a status.
var ErrNoCompleteRead = errors.New("rpc: no complete read of the key")

// Collector reads sources and publishes their complete reads. git-collector
// implements it for model.SourceGit (target branch and open pull requests),
// cloud-collector for model.SourceAWS and model.SourceCluster; a future
// reader for another cloud implements the same contract. inputs, in the server, is the only caller.
//
// A collector holds its watch list and its latest snapshots only in memory
// and has no database: after a restart it reads everything again, and the
// server ignores snapshots whose digest it already holds. Reads run on a
// polling interval and on Refresh; triggers that arrive while a read is
// queued or running are merged into it.
//
// ErrUnknownSource and ErrNoCompleteRead are the only errors with a
// meaning; callers test for them with errors.Is. The gRPC implementation
// carries them as the status codes NOT_FOUND and FAILED_PRECONDITION and
// maps those back; any other error is a failure of the call itself.
type Collector interface {
	// Watch replaces the collector's watch list with sources and streams
	// their status until ctx is done or the stream breaks, which it reports
	// as the returned error (ctx.Err() on cancellation). emit is called from
	// one goroutine at a time, for a source whenever its status changes and
	// for every source at least once per heartbeat interval. Sources of the
	// wrong kind are rejected with an error before any emit. One Watch is
	// active at a time; a new call ends the previous one.
	Watch(ctx context.Context, sources []model.Source, emit func(model.SourceStatus)) error

	// GetSnapshot returns the latest complete snapshot of key, which may be
	// newer than the last status announced. A key with no complete read
	// returns ErrNoCompleteRead; a key whose source is outside the watch
	// list returns ErrUnknownSource. A snapshot always fits the message size
	// limit both ends share and the snapshot object limit: a read over
	// either is published with no objects and model.ReasonSnapshotLimit (or
	// the renderer's model.ReasonRenderLimit), never cut short.
	GetSnapshot(ctx context.Context, key model.ReadKey) (model.Snapshot, error)

	// Refresh asks for a read of source now and returns once the request is
	// queued; the result arrives through Watch. It returns ErrUnknownSource
	// for a source outside the watch list.
	Refresh(ctx context.Context, source model.SourceID) error
}
