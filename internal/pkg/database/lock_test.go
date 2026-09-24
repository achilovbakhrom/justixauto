package database

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRunOnceExcludesOtherReplicas(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	db, err := Open(url)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	holding, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- RunOnce(ctx, db, "test-job", func(context.Context) error {
			close(holding)
			<-release
			return nil
		})
	}()
	<-holding
	// A second replica: the lock is taken, so its run is skipped at once.
	if err := RunOnce(ctx, db, "test-job", func(context.Context) error { return nil }); !errors.Is(err, ErrLocked) {
		t.Fatalf("second run: got %v, want ErrLocked", err)
	}
	// A different job is not affected.
	if err := RunOnce(ctx, db, "other-job", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("other job: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first run: %v", err)
	}
	// After the first run finished the lock is free again.
	if err := RunOnce(ctx, db, "test-job", func(context.Context) error { return nil }); err != nil {
		t.Fatalf("rerun: %v", err)
	}
}
