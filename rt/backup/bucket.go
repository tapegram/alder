package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Bucket is where backups go: an S3-compatible bucket under a prefix, or a local directory.
// Keys are slash-separated and relative to the location.
type Bucket interface {
	// Put stores the file at path as key.
	Put(ctx context.Context, key, path string) error
	// Get writes key's object to the file at path.
	Get(ctx context.Context, key, path string) error
	// List is every key under prefix, sorted.
	List(ctx context.Context, prefix string) ([]string, error)
	// String is the location, without credentials.
	String() string
}

// ErrNoObject is Get's error for a key that does not exist.
var ErrNoObject = errors.New("no such object")

// OpenBucket opens the location loc: "s3://bucket/prefix" (credentials from AWS_ACCESS_KEY_ID,
// AWS_SECRET_ACCESS_KEY and AWS_SESSION_TOKEN, the region from AWS_REGION (default
// us-east-1), an S3-compatible service's endpoint from AWS_ENDPOINT_URL), or a local
// directory: "file:///srv/backups/blog" or an absolute path.
func OpenBucket(loc string) (Bucket, error) {
	switch {
	case strings.HasPrefix(loc, "s3://"):
		return openS3(loc)
	case strings.HasPrefix(loc, "file://"):
		u, err := url.Parse(loc)
		if err != nil {
			return nil, err
		}
		return dirBucket{u.Path}, nil
	case filepath.IsAbs(loc):
		return dirBucket{loc}, nil
	}
	return nil, fmt.Errorf("backup: %q is not a backup location: write s3://bucket/prefix, or a local directory as file:///path", loc)
}

// dirBucket is a Bucket in a local directory (tests, and a mounted volume).
type dirBucket struct{ root string }

func (d dirBucket) String() string { return "file://" + d.root }

func (d dirBucket) Put(_ context.Context, key, path string) error {
	dst := filepath.Join(d.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func (d dirBucket) Get(_ context.Context, key, path string) error {
	b, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(key)))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", key, ErrNoObject)
	}
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (d dirBucket) List(_ context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(d.root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if e.IsDir() || strings.HasSuffix(p, ".tmp") {
			return nil
		}
		rel, err := filepath.Rel(d.root, p)
		if err != nil {
			return err
		}
		if k := filepath.ToSlash(rel); strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
		return nil
	})
	sort.Strings(keys)
	return keys, err
}
