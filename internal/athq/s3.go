package athq

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// parseS3URI splits an s3://bucket/key URI into its bucket and key.
func parseS3URI(uri string) (bucket, key string, err error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", "", fmt.Errorf("invalid S3 URI %q: %w", uri, err)
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("invalid S3 URI %q: scheme must be s3://", uri)
	}
	bucket = u.Host
	key = strings.TrimPrefix(u.Path, "/")
	if bucket == "" || key == "" {
		return "", "", fmt.Errorf("invalid S3 URI %q: want s3://bucket/key", uri)
	}
	return bucket, key, nil
}

// copyS3Object streams an S3 object to w. It is used to hand over the CSV that
// Athena already wrote, which keeps quoting intact and avoids paging through
// GetQueryResults.
func copyS3Object(ctx context.Context, c *clients, uri string, w io.Writer) (int64, error) {
	bucket, key, err := parseS3URI(uri)
	if err != nil {
		return 0, err
	}
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to read %s: %w", uri, err)
	}
	defer func() { _ = out.Body.Close() }()

	n, err := io.Copy(w, out.Body)
	if err != nil {
		return n, fmt.Errorf("failed to read %s: %w", uri, err)
	}
	return n, nil
}

// createFile opens dest for writing, creating any missing parent directories.
func createFile(dest string) (*os.File, error) {
	if dir := filepath.Dir(dest); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}
	f, err := os.Create(dest)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s: %w", dest, err)
	}
	return f, nil
}
