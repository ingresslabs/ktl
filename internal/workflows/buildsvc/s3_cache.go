package buildsvc

import (
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// S3CacheOptions configures the BuildKit S3 cache backend.
type S3CacheOptions struct {
	Ref          string
	Region       string
	Name         string
	Mode         string
	EndpointURL  string
	UsePathStyle bool
}

func (o S3CacheOptions) enabled() bool {
	return strings.TrimSpace(o.Ref) != ""
}

// ApplyS3Cache appends BuildKit S3 cache import/export specs when configured.
func ApplyS3Cache(cacheFrom, cacheTo []string, opts S3CacheOptions, defaultName string) ([]string, []string, error) {
	if !opts.enabled() {
		return cacheFrom, cacheTo, nil
	}
	from, to, err := S3CacheSpecs(opts, defaultName)
	if err != nil {
		return nil, nil, err
	}
	return append(cacheFrom, from), append(cacheTo, to), nil
}

// S3CacheSpecs converts the first-class S3 cache options into BuildKit cache specs.
func S3CacheSpecs(opts S3CacheOptions, defaultName string) (string, string, error) {
	bucket, prefix, err := parseS3CacheRef(opts.Ref)
	if err != nil {
		return "", "", err
	}
	region := strings.TrimSpace(opts.Region)
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_REGION"))
	}
	if region == "" {
		region = strings.TrimSpace(os.Getenv("AWS_DEFAULT_REGION"))
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = defaultName
	}
	name = sanitizeS3CacheName(name)
	if name == "" {
		name = "buildkit"
	}
	mode := strings.TrimSpace(strings.ToLower(opts.Mode))
	if mode == "" {
		mode = "max"
	}
	if mode != "min" && mode != "max" {
		return "", "", fmt.Errorf("invalid s3 cache mode %q (want min or max)", opts.Mode)
	}

	attrs := []string{"type=s3", "bucket=" + bucket}
	if region != "" {
		attrs = append(attrs, "region="+region)
	}
	if prefix != "" {
		attrs = append(attrs, "prefix="+prefix)
	}
	attrs = append(attrs, "name="+name)
	if endpoint := strings.TrimSpace(opts.EndpointURL); endpoint != "" {
		attrs = append(attrs, "endpoint_url="+endpoint)
	}
	if opts.UsePathStyle {
		attrs = append(attrs, "use_path_style=true")
	}
	for _, attr := range attrs {
		if strings.ContainsAny(attr, ",\n\r") {
			return "", "", fmt.Errorf("s3 cache attribute %q contains an unsupported separator", attr)
		}
	}

	from := strings.Join(attrs, ",")
	to := strings.Join(append(append([]string(nil), attrs...), "mode="+mode), ",")
	return from, to, nil
}

func DefaultS3CacheName(contextDir string, tags []string) string {
	for _, tag := range tags {
		if strings.TrimSpace(tag) != "" {
			return tag
		}
	}
	contextDir = strings.TrimSpace(contextDir)
	if contextDir == "" {
		contextDir = "."
	}
	base := filepath.Base(filepath.Clean(contextDir))
	if base == "." || base == string(filepath.Separator) || base == "" {
		return "buildkit"
	}
	return base
}

func parseS3CacheRef(ref string) (bucket, prefix string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", fmt.Errorf("s3 cache ref cannot be empty")
	}
	if strings.HasPrefix(ref, "s3://") {
		u, err := url.Parse(ref)
		if err != nil {
			return "", "", fmt.Errorf("parse s3 cache ref: %w", err)
		}
		bucket = strings.TrimSpace(u.Host)
		prefix = strings.TrimPrefix(u.EscapedPath(), "/")
		if decoded, derr := url.PathUnescape(prefix); derr == nil {
			prefix = decoded
		}
	} else {
		bucket, prefix, _ = strings.Cut(ref, "/")
		bucket = strings.TrimSpace(bucket)
	}
	if bucket == "" {
		return "", "", fmt.Errorf("s3 cache ref %q is missing bucket", ref)
	}
	if strings.ContainsAny(bucket, ",/\n\r") {
		return "", "", fmt.Errorf("s3 cache bucket %q is invalid", bucket)
	}
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimPrefix(path.Clean("/"+prefix), "/")
	if prefix == "." {
		prefix = ""
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return bucket, prefix, nil
}

func sanitizeS3CacheName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r == '/' || r == ':' || r == '@':
			b.WriteByte('-')
		case r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}
