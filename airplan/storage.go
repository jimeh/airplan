package airplan

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// storage wraps the S3-compatible client used for uploads (SPEC.md §5).
// It stays unexported: consumers upload through Client.
type storage struct {
	bucket string
	client *s3.Client
}

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

	return &storage{bucket: cfg.Bucket, client: client}, nil
}

// object is a single object to upload.
type object struct {
	Key         string
	Body        []byte
	ContentType string
	// Metadata becomes x-amz-meta-* headers (e.g. "title" for
	// list --remote titles, SPEC.md §5).
	Metadata map[string]string
}

// put uploads one object with
// Cache-Control: public, max-age=31536000, immutable (SPEC.md §5).
func (s *storage) put(ctx context.Context, obj object) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(s.bucket),
		Key:          aws.String(obj.Key),
		Body:         bytes.NewReader(obj.Body),
		ContentType:  aws.String(obj.ContentType),
		CacheControl: aws.String("public, max-age=31536000, immutable"),
		Metadata:     obj.Metadata,
	})
	if err != nil {
		return fmt.Errorf("airplan: put object %q: %w", obj.Key, err)
	}
	return nil
}

// PublicURL assembles the public URL for an object key:
// <public_base_url>/<key> when public_base_url is set, else path-style
// <endpoint>/<bucket>/<key> with fallback=true so the caller can warn
// that the URL may not be publicly reachable (SPEC.md §7, §8).
func PublicURL(cfg *Config, key string) (url string, fallback bool) {
	if cfg.PublicBaseURL != "" {
		return strings.TrimRight(cfg.PublicBaseURL, "/") + "/" + key, false
	}

	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	return endpoint + "/" + cfg.Bucket + "/" + key, true
}
