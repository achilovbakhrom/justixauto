package repository

import (
	"bytes"
	"context"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config selects the bucket. Credentials come from the standard AWS chain
// (environment, shared profile, IAM role). Endpoint and PathStyle are for
// S3-compatible stores such as MinIO; leave them empty for AWS.
type S3Config struct {
	Bucket    string
	Region    string
	Prefix    string // key prefix inside the bucket, e.g. "documents/"
	Endpoint  string
	PathStyle bool
	// SSE requests server-side encryption ("AES256" or "aws:kms"); empty
	// relies on the bucket's default encryption.
	SSE string
}

// S3Storage keeps file bytes in a private S3 bucket. Objects are never public;
// every read goes through the API's access checks.
type S3Storage struct {
	client *s3.Client
	cfg    S3Config
}

func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.PathStyle
	})
	return &S3Storage{client: client, cfg: cfg}, nil
}

func (s *S3Storage) key(k string) *string { return aws.String(s.cfg.Prefix + k) }

func (s *S3Storage) Put(ctx context.Context, key string, data []byte, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.cfg.Bucket), Key: s.key(key), Body: bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))), ContentType: aws.String(contentType),
		IfNoneMatch: aws.String("*"),
	} // stored bytes never change
	if s.cfg.SSE != "" {
		in.ServerSideEncryption = types.ServerSideEncryption(s.cfg.SSE)
	}
	_, err := s.client.PutObject(ctx, in)
	return err
}

func (s *S3Storage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.cfg.Bucket), Key: s.key(key)})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.cfg.Bucket), Key: s.key(key)})
	return err
}

// EnsureBucket creates the bucket if it does not exist (development, tests).
func (s *S3Storage) EnsureBucket(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.cfg.Bucket)}); err == nil {
		return nil
	}
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(s.cfg.Bucket)})
	return err
}
