package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	neturl "net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// storage wraps the S3-compatible client used for uploads (SPEC.md §5).
// It stays unexported: consumers upload through Client.
type storage struct {
	bucket string
	client *s3.Client
	creds  aws.CredentialsProvider
}

var errObjectNotFound = errors.New("object not found")

// newStorage builds the S3 client from cfg: custom endpoint support,
// path-style addressing when a custom endpoint is set (R2, MinIO),
// region defaulting to "auto", and request checksum calculation set to
// when-required for R2 compatibility. Credentials fall back per
// SPEC.md §7: explicit config values, else the standard AWS chain.
func newStorage(ctx context.Context, cfg *Config) (*storage, error) {
	region := cfg.Region
	if region == "" {
		region = "auto"
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(region),
		awsconfig.WithRequestChecksumCalculation(
			aws.RequestChecksumCalculationWhenRequired,
		),
		awsconfig.WithResponseChecksumValidation(
			aws.ResponseChecksumValidationWhenRequired,
		),
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		provider := credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		)
		opts = append(opts, awsconfig.WithCredentialsProvider(provider))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("airplan: load storage config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		}
	})

	return &storage{
		bucket: cfg.Bucket,
		client: client,
		creds:  awsCfg.Credentials,
	}, nil
}

// checkCredentials resolves the credential chain once, up front, so a
// missing-credentials failure is a clear startup error instead of a
// PutObject failure after input has already been consumed (SPEC.md §7
// startup validation).
func (s *storage) checkCredentials(ctx context.Context) error {
	if s.creds == nil {
		return errors.New("airplan: no credential provider configured")
	}
	if _, err := s.creds.Retrieve(ctx); err != nil {
		return fmt.Errorf(
			"airplan: no usable credentials: %w; set access_key_id / "+
				"secret_access_key in the config file, "+
				"AIRPLAN_ACCESS_KEY_ID / AIRPLAN_SECRET_ACCESS_KEY env "+
				"vars, or configure the standard AWS credential chain",
			err,
		)
	}
	return nil
}

// object is a single object to upload.
type object struct {
	Key         string
	Body        []byte
	ContentType string
	// Metadata becomes x-amz-meta-* headers.
	Metadata map[string]string
}

// put uploads one object with Cache-Control: no-store (SPEC.md §5).
func (s *storage) put(ctx context.Context, obj object) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(obj.Key),
		Body:         bytes.NewReader(obj.Body),
		ContentType:  aws.String(obj.ContentType),
		CacheControl: aws.String("no-store"),
		Metadata:     obj.Metadata,
	})
	if err != nil {
		return fmt.Errorf("airplan: put object %q: %w", obj.Key, err)
	}
	return nil
}

// getBytes fetches an object while retaining at most limit+1 bytes when limit
// is positive. The extra byte lets strict callers identify oversized bodies.
func (s *storage) getBytes(
	ctx context.Context, key string, limit int64,
) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, fmt.Errorf("airplan: get object %q: %w",
				key, errObjectNotFound)
		}
		return nil, fmt.Errorf("airplan: get object %q: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()

	var reader io.Reader = out.Body
	if limit > 0 {
		reader = io.LimitReader(reader, limit+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("airplan: read object %q: %w", key, err)
	}
	return body, nil
}

// deleteMarker removes the ownership marker in a dedicated final request.
func (s *storage) deleteMarker(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("airplan: delete marker %q: %w", key, err)
	}
	return nil
}

// PublicURL assembles the public URL for an object key:
// <public_base_url>/<key> when public_base_url is set, else path-style
// <endpoint>/<bucket>/<key> with fallback=true so the caller can warn
// that the URL may not be publicly reachable (SPEC.md §7, §8).
func PublicURL(cfg *Config, key string) (url string, fallback bool) {
	if cfg.PublicBaseURL != "" {
		return appendURLPath(cfg.PublicBaseURL, key), false
	}

	return appendURLPath(cfg.Endpoint, cfg.Bucket+"/"+key), true
}

func appendURLPath(base, path string) string {
	u, err := neturl.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + "/" + escapeObjectKey(path)
	}
	basePath := strings.TrimRight(u.Path, "/")
	baseRawPath := strings.TrimRight(u.EscapedPath(), "/")
	u.Path = basePath + "/" + path
	u.RawPath = baseRawPath + "/" +
		escapeObjectKey(path)
	return u.String()
}

func escapeObjectKey(key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = neturl.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}

// objectInfo describes one listed object (SPEC.md §9).
type objectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// listKeys returns every object under prefix, paginating as needed.
func (s *storage) listKeys(
	ctx context.Context, prefix string,
) ([]objectInfo, error) {
	var out []objectInfo
	p := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"airplan: list objects %q: %w", prefix, err)
		}
		for _, o := range page.Contents {
			out = append(out, objectInfo{
				Key:          aws.ToString(o.Key),
				Size:         aws.ToInt64(o.Size),
				LastModified: aws.ToTime(o.LastModified),
			})
		}
	}
	return out, nil
}

// deleteKeys removes objects in DeleteObjects batches (1000 max per
// call). The first per-object failure is returned as an error so
// callers can leave the upload un-tombstoned and retry (SPEC.md §9).
func (s *storage) deleteKeys(ctx context.Context, keys []string) error {
	const batchSize = 1000
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		ids := make([]types.ObjectIdentifier, 0, end-start)
		for _, k := range keys[start:end] {
			ids = append(ids, types.ObjectIdentifier{Key: aws.String(k)})
		}

		out, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("airplan: delete objects: %w", err)
		}
		if len(out.Errors) > 0 {
			e := out.Errors[0]
			return fmt.Errorf("airplan: delete %q: %s",
				aws.ToString(e.Key), aws.ToString(e.Message))
		}
	}
	return nil
}
