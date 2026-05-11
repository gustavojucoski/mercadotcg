package upload

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Provider stores files in an AWS S3 bucket with public-read ACL.
type S3Provider struct {
	client    *s3.Client
	bucket    string
	region    string
	customURL string // e.g. "https://images.mercadotcg.com.br" (CloudFront). Empty → default S3 URL.
}

// NewS3 creates an S3Provider. Credentials are loaded via the AWS default
// credential chain (env vars, shared config, EC2/ECS IAM roles) — no hardcoding.
func NewS3(bucket, region, customURL string) (*S3Provider, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("upload: carregar config AWS: %w", err)
	}
	return &S3Provider{
		client:    s3.NewFromConfig(cfg),
		bucket:    bucket,
		region:    region,
		customURL: customURL,
	}, nil
}

// Put uploads r to S3 under key with public-read ACL.
// contentType is forwarded as the S3 Content-Type so browsers serve the file correctly.
func (p *S3Provider) Put(ctx context.Context, key string, r io.Reader, contentType string) (string, error) {
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(p.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType),
		ACL:         types.ObjectCannedACLPublicRead,
	})
	if err != nil {
		return "", fmt.Errorf("upload: s3 put %s: %w", key, err)
	}
	return p.PublicURL(key), nil
}

// PublicURL returns the public URL for key.
// Uses customURL as the origin prefix when set; falls back to the standard S3 URL.
func (p *S3Provider) PublicURL(key string) string {
	if p.customURL != "" {
		return p.customURL + "/" + key
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", p.bucket, p.region, key)
}

// Exists uses HeadObject to check whether key is present in the bucket.
// Returns (false, nil) for a 404; (false, err) for any other error.
func (p *S3Provider) Exists(ctx context.Context, key string) (bool, error) {
	_, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, nil
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return false, nil
	}
	return false, fmt.Errorf("upload: s3 head %s: %w", key, err)
}
