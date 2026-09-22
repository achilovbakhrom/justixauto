package documents_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"

	"justixauto/internal/modules/documents"
)

func TestS3Storage(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TEST_S3_ENDPOINT (bash tools/test-go.sh starts MinIO)")
	}
	ctx := context.Background()
	s, err := documents.NewS3Storage(ctx, documents.S3Config{Bucket: "justixauto-test", Region: "us-east-1",
		Prefix: "adapter/", Endpoint: endpoint, PathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	if err := s.Put(ctx, key, []byte("first"), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, key, []byte("second"), "application/pdf"); err == nil {
		t.Fatal("stored bytes must never be overwritten")
	}
	r, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if !bytes.Equal(got, []byte("first")) {
		t.Fatalf("read back %q", got)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Open(ctx, key); err == nil {
		t.Fatal("deleted object still readable")
	}
}
