package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3 stores objects in an S3-compatible bucket (AWS S3, MinIO, ...).
type S3 struct {
	client *s3.Client
	bucket string
	prefix string
}

// NewS3FromEnv builds an S3 store from the environment:
//
//	S3_BUCKET   (required)
//	S3_PREFIX   (optional key prefix, e.g. "exports/")
//	S3_ENDPOINT (optional, for MinIO/localstack; enables path-style addressing)
//	AWS_REGION / AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY via the standard SDK chain
func NewS3FromEnv(ctx context.Context) (*S3, error) {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET is required when EXPORT_STORAGE=s3")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	endpoint := os.Getenv("S3_ENDPOINT")
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // MinIO and most local S3s need path-style
		}
	})
	prefix := os.Getenv("S3_PREFIX")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &S3{client: client, bucket: bucket, prefix: prefix}, nil
}

func (s *S3) Save(ctx context.Context, key string, body io.Reader) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.prefix + key),
		Body:   body,
	})
	return err
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.prefix + key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// FromEnv selects the storage backend: EXPORT_STORAGE=s3 for object storage,
// anything else (default) for local disk under EXPORT_DIR.
func FromEnv(ctx context.Context) (Storage, error) {
	if os.Getenv("EXPORT_STORAGE") == "s3" {
		return NewS3FromEnv(ctx)
	}
	dir := os.Getenv("EXPORT_DIR")
	if dir == "" {
		dir = "./tmp/exports"
	}
	return NewLocal(dir)
}
