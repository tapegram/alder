package backup

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

// s3Bucket is a Bucket in an S3-compatible service: objects are PUT, GET and listed
// (ListObjectsV2) with requests signed by AWS Signature Version 4. With AWS_ENDPOINT_URL (R2,
// MinIO, …) the bucket is addressed by path (endpoint/bucket/key); without it, AWS's
// virtual-hosted form (bucket.s3.region.amazonaws.com/key).
type s3Bucket struct {
	bucket, prefix string
	endpoint       string // scheme://host
	pathStyle      bool
	creds          s3Creds
	client         *http.Client
}

type s3Creds struct{ access, secret, token, region string }

func openS3(loc string) (*s3Bucket, error) {
	rest := strings.TrimPrefix(loc, "s3://")
	bucket, prefix, _ := strings.Cut(rest, "/")
	if bucket == "" {
		return nil, fmt.Errorf("backup: %q names no bucket", loc)
	}
	c := s3Creds{access: os.Getenv("AWS_ACCESS_KEY_ID"), secret: os.Getenv("AWS_SECRET_ACCESS_KEY"), token: os.Getenv("AWS_SESSION_TOKEN"), region: os.Getenv("AWS_REGION")}
	if c.region == "" {
		c.region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if c.region == "" {
		c.region = "us-east-1"
	}
	if c.access == "" || c.secret == "" {
		return nil, fmt.Errorf("backup: %s needs AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the environment (name them in `deploy.requires`)", loc)
	}
	b := &s3Bucket{bucket: bucket, prefix: strings.Trim(prefix, "/"), creds: c, client: &http.Client{Timeout: 5 * time.Minute}}
	if ep := os.Getenv("AWS_ENDPOINT_URL"); ep != "" {
		b.endpoint, b.pathStyle = strings.TrimRight(ep, "/"), true
	} else {
		b.endpoint = "https://" + bucket + ".s3." + c.region + ".amazonaws.com"
	}
	return b, nil
}

func (b *s3Bucket) String() string {
	if b.prefix == "" {
		return "s3://" + b.bucket
	}
	return "s3://" + b.bucket + "/" + b.prefix
}

// objectPath is the request path of key.
func (b *s3Bucket) objectPath(key string) string {
	k := key
	if b.prefix != "" {
		k = b.prefix + "/" + key
	}
	if b.pathStyle {
		return "/" + b.bucket + "/" + k
	}
	return "/" + k
}

func (b *s3Bucket) Put(ctx context.Context, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", b.endpoint+uriEncode(b.objectPath(key), false), f)
	if err != nil {
		return err
	}
	req.ContentLength = n
	resp, err := b.do(req, hex.EncodeToString(h.Sum(nil)))
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (b *s3Bucket) Get(ctx context.Context, key, path string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", b.endpoint+uriEncode(b.objectPath(key), false), nil)
	if err != nil {
		return err
	}
	resp, err := b.do(req, emptySHA256)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (b *s3Bucket) List(ctx context.Context, prefix string) ([]string, error) {
	full := prefix
	if b.prefix != "" {
		full = b.prefix + "/" + prefix
	}
	var keys []string
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "prefix": {full}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		p := "/"
		if b.pathStyle {
			p = "/" + b.bucket
		}
		req, err := http.NewRequestWithContext(ctx, "GET", b.endpoint+p+"?"+canonicalQuery(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := b.do(req, emptySHA256)
		if err != nil {
			return nil, err
		}
		var page struct {
			Contents []struct {
				Key string `xml:"Key"`
			} `xml:"Contents"`
			IsTruncated           bool   `xml:"IsTruncated"`
			NextContinuationToken string `xml:"NextContinuationToken"`
		}
		err = xml.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("backup: listing %s: %w", b, err)
		}
		for _, c := range page.Contents {
			k := c.Key
			if b.prefix != "" {
				k = strings.TrimPrefix(k, b.prefix+"/")
			}
			keys = append(keys, k)
		}
		if !page.IsTruncated || page.NextContinuationToken == "" {
			break
		}
		token = page.NextContinuationToken
	}
	sort.Strings(keys)
	return keys, nil
}

// do signs req (its payload's SHA-256 is payloadHash) and sends it; a status other than 2xx
// is an error with the service's message (a 404 wraps ErrNoObject).
func (b *s3Bucket) do(req *http.Request, payloadHash string) (*http.Response, error) {
	sign(req, b.creds, payloadHash, time.Now())
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backup: %s %s: %w", req.Method, b, err)
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		var e struct {
			Code    string `xml:"Code"`
			Message string `xml:"Message"`
		}
		xml.Unmarshal(body, &e)
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("backup: %s %s: %w (%s)", req.Method, req.URL.Path, ErrNoObject, e.Code)
		}
		return nil, fmt.Errorf("backup: %s %s at %s: %s %s: %s", req.Method, req.URL.Path, b, resp.Status, e.Code, e.Message)
	}
	return resp, nil
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign adds AWS Signature Version 4 headers to req for service s3: x-amz-date,
// x-amz-content-sha256 (payloadHash), x-amz-security-token with a session token, and
// Authorization, signing every header that is set plus host.
func sign(req *http.Request, c s3Creds, payloadHash string, now time.Time) {
	amzDate := now.UTC().Format("20060102T150405Z")
	date := amzDate[:8]
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if c.token != "" {
		req.Header.Set("x-amz-security-token", c.token)
	}
	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		headers[strings.ToLower(k)] = strings.TrimSpace(strings.Join(v, ","))
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{req.Method, path, canonicalQuery(req.URL.Query()), canonHeaders.String(), signed, payloadHash}, "\n")
	scope := date + "/" + c.region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])
	key := hmacSHA256([]byte("AWS4"+c.secret), date)
	key = hmacSHA256(key, c.region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.access+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

// canonicalQuery is q sorted by key, each key and value URI-encoded (SigV4's form).
func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vs := append([]string{}, q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	return strings.Join(parts, "&")
}

// uriEncode percent-encodes every byte of s but the unreserved ones (A-Z a-z 0-9 - . _ ~),
// and "/" unless encodeSlash.
func uriEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
