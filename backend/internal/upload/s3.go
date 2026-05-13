package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
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
// The body is buffered in memory so Content-Length can be set — S3 rejects streaming
// uploads without a known size (HTTP 411 MissingContentLength).
func (p *S3Provider) Put(ctx context.Context, key string, r io.Reader, contentType string) (string, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("upload: ler body para %s: %w", key, err)
	}
	// No ACL field: bucket uses a public-read bucket policy instead of per-object ACLs.
	// New S3 buckets disable ACLs by default (Object Ownership = bucket owner enforced).
	_, err = p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(p.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(body),
		ContentLength: aws.Int64(int64(len(body))),
		ContentType:   aws.String(contentType),
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
// Returns (false, nil) for a 404 or 403; (false, err) for any other error.
//
// S3 returns 403 Forbidden (instead of 404) when the caller lacks s3:ListBucket
// on the bucket — a documented AWS behaviour. Treating 403 as "not found" lets
// PutObject be the real arbiter of write permission, so image downloads are not
// incorrectly aborted before the upload even begins.
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
	// S3 returns 403 instead of 404 when the IAM caller lacks s3:ListBucket.
	// Treat 403 the same as 404: object is presumed absent, allow PutObject to proceed.
	var httpErr *awshttp.ResponseError
	if errors.As(err, &httpErr) && httpErr.HTTPStatusCode() == http.StatusForbidden {
		return false, nil
	}
	return false, fmt.Errorf("upload: s3 head %s: %w", key, err)
}
