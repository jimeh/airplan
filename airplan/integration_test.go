//go:build integration

package airplan

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/testcontainers/testcontainers-go"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

// TestIntegrationRoundTrip uploads through the real pipeline against a
// live S3-compatible server (MinIO, managed by testcontainers) and
// verifies bytes and headers by fetching the objects back. Excluded
// from plain `go test ./...` by the integration build tag; run via
// `mise run test-integration`.
func TestIntegrationRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(
		context.Background(), 5*time.Minute,
	)
	defer cancel()

	minioC, err := tcminio.Run(ctx, "minio/minio:latest")
	testcontainers.CleanupContainer(t, minioC)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := minioC.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Endpoint:        "http://" + endpoint,
		Bucket:          "airplan-test",
		Region:          "us-east-1",
		AccessKeyID:     minioC.Username,
		SecretAccessKey: minioC.Password,
	}

	st, err := newStorage(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(cfg.Bucket),
	})
	if err != nil {
		t.Fatal(err)
	}

	client, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	src := "# Integration Plan\n\nSome **bold** text.\n\n" +
		"```go\nfmt.Println(\"hi\")\n```\n"
	res, err := client.Upload(ctx, Input{
		Reader: strings.NewReader(src),
		Name:   "integration-plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}

	if res.Title != "Integration Plan" {
		t.Errorf("Title = %q, want %q", res.Title, "Integration Plan")
	}
	if !strings.HasSuffix(res.Key, "/integration-plan.html") {
		t.Errorf("unexpected page key %q", res.Key)
	}
	if !strings.HasSuffix(res.SourceKey, "/integration-plan.md") {
		t.Errorf("unexpected source key %q", res.SourceKey)
	}

	page := getObject(ctx, t, st, res.Key)
	if page.contentType != "text/html; charset=utf-8" {
		t.Errorf("page Content-Type = %q", page.contentType)
	}
	if page.cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("page Cache-Control = %q", page.cacheControl)
	}
	if page.metaTitle != "Integration Plan" {
		t.Errorf("page x-amz-meta-title = %q", page.metaTitle)
	}
	if int64(len(page.body)) != res.Bytes {
		t.Errorf("page bytes = %d, Result.Bytes = %d",
			len(page.body), res.Bytes)
	}
	if !strings.Contains(string(page.body), "Some <strong>bold</strong>") {
		t.Error("page body missing rendered markdown")
	}
	if !strings.Contains(string(page.body),
		`href="./integration-plan.md" download`) {
		t.Error("page missing download link to sibling source")
	}
	if !strings.Contains(string(page.body),
		`class="raw" href="./integration-plan.md"`) {
		t.Error("page missing raw link to sibling source")
	}

	source := getObject(ctx, t, st, res.SourceKey)
	if string(source.body) != src {
		t.Error("source object bytes differ from input")
	}
	if source.contentType != "text/markdown; charset=utf-8" {
		t.Errorf("source Content-Type = %q", source.contentType)
	}
	if source.cacheControl != "public, max-age=31536000, immutable" {
		t.Errorf("source Cache-Control = %q", source.cacheControl)
	}
}

type fetchedObject struct {
	body         []byte
	contentType  string
	cacheControl string
	metaTitle    string
}

func getObject(
	ctx context.Context,
	t *testing.T,
	st *storage,
	key string,
) fetchedObject {
	t.Helper()

	out, err := st.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(st.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	defer func() { _ = out.Body.Close() }()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		t.Fatalf("read %s: %v", key, err)
	}

	return fetchedObject{
		body:         body,
		contentType:  aws.ToString(out.ContentType),
		cacheControl: aws.ToString(out.CacheControl),
		metaTitle:    out.Metadata["title"],
	}
}
