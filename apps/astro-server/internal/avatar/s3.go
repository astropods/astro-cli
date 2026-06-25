package avatar

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Backend stores avatar images in an S3 bucket.
type S3Backend struct {
	client *s3.Client
	bucket string
}

// NewS3Backend creates a new S3-backed storage backend.
func NewS3Backend(client *s3.Client, bucket string) *S3Backend {
	return &S3Backend{client: client, bucket: bucket}
}

func (b *S3Backend) Read(ctx context.Context, key string) ([]byte, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3 get %s: %w", key, err)
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

// avatarCacheControl is the Cache-Control header set on all avatar objects.
// Browsers and the CDN treat a cached copy as fresh for 1 day, then serve it
// while revalidating in the background (stale-while-revalidate) for up to 7
// days. S3 automatically generates ETags so conditional revalidation returns
// 304 when the content hasn't changed. On the versioned hot paths freshness
// comes from the changing `?v` token rather than the TTL — a new image gets a
// new URL and is fetched immediately — while any unversioned surface
// self-heals within a day.
const avatarCacheControl = "public, max-age=86400, stale-while-revalidate=604800"

func (b *S3Backend) Write(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := b.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(b.bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String(avatarCacheControl),
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

func (b *S3Backend) Copy(ctx context.Context, src, dst string) error {
	_, err := b.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            aws.String(b.bucket),
		CopySource:        aws.String(b.bucket + "/" + src),
		Key:               aws.String(dst),
		CacheControl:      aws.String(avatarCacheControl),
		MetadataDirective: types.MetadataDirectiveReplace,
	})
	if err != nil {
		return fmt.Errorf("s3 copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

func (b *S3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3 delete %s: %w", key, err)
	}
	return nil
}

func (b *S3Backend) Exists(ctx context.Context, key string) (bool, error) {
	_, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("s3 head %s: %w", key, err)
	}
	return true, nil
}

// ContentType returns the Content-Type of the object at key.
// Returns an error if the object does not exist.
func (b *S3Backend) ContentType(ctx context.Context, key string) (string, error) {
	out, err := b.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return "", fmt.Errorf("s3 head %s: %w", key, err)
	}
	if out.ContentType != nil {
		return *out.ContentType, nil
	}
	return "", nil
}
