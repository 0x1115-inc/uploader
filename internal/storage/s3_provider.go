package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type S3Provider struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

func NewS3(cfg S3Config) (*S3Provider, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	endpoint := fmt.Sprintf("%s://%s", scheme, cfg.Endpoint)

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})
	presigner := s3.NewPresignClient(client)

	provider := &S3Provider{client: client, presigner: presigner, bucket: cfg.Bucket}
	if err := provider.ensureBucket(context.Background(), region); err != nil {
		return nil, err
	}
	return provider, nil
}

func (s *S3Provider) ensureBucket(ctx context.Context, region string) error {
	_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
		CreateBucketConfiguration: &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		},
	})
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.ErrorCode() == "BucketAlreadyOwnedByYou" || apiErr.ErrorCode() == "BucketAlreadyExists" {
		return nil
	}
	return err
}

func (s *S3Provider) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	out, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.ETag), nil
}

func (s *S3Provider) GetSignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}

	out, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

var _ Provider = (*S3Provider)(nil)
