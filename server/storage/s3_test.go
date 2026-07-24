package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// The S3 backend is exercised against a real S3 API — Minio locally, or any
// endpoint an operator points it at — because the parts that matter (multipart
// upload, range reads, the listing Usage sums) have no meaningful fake.
//
//	docker run -d --name minio -p 9000:9000 minio/minio server /data
//	S3_TEST_ENDPOINT=http://127.0.0.1:9000 go test ./server/storage/...
//
// Without S3_TEST_ENDPOINT the whole file skips, so CI stays offline.
func s3TestStorage(t *testing.T) *S3Storage {
	t.Helper()

	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("set S3_TEST_ENDPOINT to run the S3 backend tests")
	}

	accessKey := envOr("S3_TEST_ACCESS_KEY", "minioadmin")
	secretKey := envOr("S3_TEST_SECRET_KEY", "minioadmin")
	bucket := envOr("S3_TEST_BUCKET", "sendto-test")

	ctx := t.Context()

	// The backend does not create its bucket, so the test does.
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		t.Fatal(err)
	}

	admin := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})

	if _, err := admin.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
		// Already existing is the normal case on a second run.
		t.Logf("CreateBucket: %v", err)
	}

	store, err := NewS3Storage(ctx, accessKey, secretKey, bucket, 0, "us-east-1", endpoint, false, true,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func TestS3StorageRoundTripAndUsage(t *testing.T) {
	store := s3TestStorage(t)
	ctx := t.Context()

	token := "quota" + randomSuffix()
	payload := bytes.Repeat([]byte("s"), 4096)

	before, err := store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Put(ctx, token, "a.bin", bytes.NewReader(payload), "application/octet-stream", uint64(len(payload))); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	length, err := store.Head(ctx, token, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if length != uint64(len(payload)) {
		t.Errorf("Head = %d, want %d", length, len(payload))
	}

	reader, _, err := store.Get(ctx, token, "a.bin", ParseRange("bytes=4000-"))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(reader)
	_ = reader.Close()

	if len(got) != 96 {
		t.Errorf("ranged Get returned %d bytes, want 96", len(got))
	}

	after, err := store.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if after-before != uint64(len(payload)) {
		t.Errorf("Usage grew by %d, want %d — the total-size quota would be wrong by that much",
			after-before, len(payload))
	}
}

func TestS3PurgeDeletesExpiredObjects(t *testing.T) {
	store := s3TestStorage(t)
	ctx := t.Context()

	token := "purge" + randomSuffix()

	if err := store.Put(ctx, token, "a.bin", bytes.NewReader([]byte("old")), "text/plain", 3); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	// Everything in the bucket is older than a cutoff in the future, so this
	// sweep must empty it. Before this backend had a real Purge it returned nil
	// here and deleted nothing, and --purge-days was a promise the server never
	// kept.
	if err := store.Purge(ctx, -time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Head(ctx, token, "a.bin"); err == nil {
		t.Fatal("object survived a purge that covered it")
	}
}

func TestS3PurgeKeepsFreshObjects(t *testing.T) {
	store := s3TestStorage(t)
	ctx := t.Context()

	token := "fresh" + randomSuffix()

	if err := store.Put(ctx, token, "a.bin", bytes.NewReader([]byte("new")), "text/plain", 3); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Delete(ctx, token, "a.bin") }()

	if err := store.Purge(ctx, 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Head(ctx, token, "a.bin"); err != nil {
		t.Fatalf("purge deleted an object that had not expired: %v", err)
	}
}

// randomSuffix keeps parallel runs against a shared bucket from colliding.
func randomSuffix() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "fixed"
	}
	return hex.EncodeToString(b[:])
}
