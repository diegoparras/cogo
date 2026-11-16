package artifact

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// S3Store speaks the S3 REST API (path-style) to Cloudflare R2. It implements
// just the four object operations COGO needs — PUT/GET/HEAD/DELETE of a single
// key — signed with AWS Signature V4 by hand, so the module stays dependency-free
// (no aws-sdk). Keys are content hashes, so no listing or querying is required
// and the canonical request has no query string and only three signed headers.
type S3Store struct {
	endpoint  string // https://<account>.r2.cloudflarestorage.com
	bucket    string
	prefix    string // key namespace inside the bucket, e.g. "artifacts/"
	accessKey string
	secretKey string
	region    string // "auto" for R2
	client    *http.Client
}

func newHTTPClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

func (s *S3Store) objURL(sha string) string {
	return strings.TrimRight(s.endpoint, "/") + "/" + s.bucket + "/" + s.prefix + sha
}

func (s *S3Store) Backend() string { return "r2" }

func (s *S3Store) Put(ctx context.Context, content []byte, contentType string) (string, error) {
	sha := Sha256Hex(content)
	if ok, err := s.Has(ctx, sha); err == nil && ok {
		return sha, nil // dedup: identical bytes already stored
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objURL(sha), bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = int64(len(content))
	// The payload hash for a PUT is the object's own key — no extra hashing.
	s.sign(req, sha)
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(resp)
	if resp.StatusCode/100 != 2 {
		return "", s3err("PUT", sha, resp)
	}
	return sha, nil
}

func (s *S3Store) Get(ctx context.Context, sha string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objURL(sha), nil)
	if err != nil {
		return nil, err
	}
	s.sign(req, emptyHash)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode/100 != 2 {
		return nil, s3err("GET", sha, resp)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if Sha256Hex(b) != sha {
		return nil, fmt.Errorf("artifact: integrity check failed for %s (content does not match key)", sha)
	}
	return b, nil
}

func (s *S3Store) Has(ctx context.Context, sha string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, s.objURL(sha), nil)
	if err != nil {
		return false, err
	}
	s.sign(req, emptyHash)
	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer drain(resp)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	case resp.StatusCode/100 == 2:
		return true, nil
	default:
		return false, s3err("HEAD", sha, resp)
	}
}

func (s *S3Store) Delete(ctx context.Context, sha string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objURL(sha), nil)
	if err != nil {
		return err
	}
	s.sign(req, emptyHash)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer drain(resp)
	// S3 DELETE is idempotent: 204 whether or not the key existed.
	if resp.StatusCode/100 != 2 && resp.StatusCode != http.StatusNotFound {
		return s3err("DELETE", sha, resp)
	}
	return nil
}

// sign applies AWS Signature V4 to req in place. payloadHash is the hex SHA-256
// of the body (emptyHash for bodyless requests, the object key for a PUT).
func (s *S3Store) sign(req *http.Request, payloadHash string) {
	now := nowUTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	host := req.URL.Host

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	const signedHeaders = "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := "host:" + host + "\n" +
		"x-amz-content-sha256:" + payloadHash + "\n" +
		"x-amz-date:" + amzDate + "\n"

	canonicalRequest := strings.Join([]string{
		req.Method,
		uriEncodePath(req.URL.Path),
		req.URL.RawQuery, // empty for single-object ops
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	scope := dateStamp + "/" + s.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hashHex([]byte(canonicalRequest)),
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, s.region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.accessKey, scope, signedHeaders, signature))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// uriEncodePath encodes each path segment per RFC3986 while preserving the
// slashes, as SigV4's canonical URI requires. COGO's keys are hex so nothing is
// actually escaped, but the bucket/prefix segments are encoded correctly too.
func uriEncodePath(p string) string {
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		segs[i] = uriEncodeSegment(seg)
	}
	return strings.Join(segs, "/")
}

func uriEncodeSegment(s string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// drain reads and closes a response body so the connection can be reused.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// s3err builds an error from a non-2xx response, including a snippet of the
// XML error body R2 returns (Code/Message).
func s3err(op, sha string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.TrimSpace(string(body))
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("artifact: %s %s failed: %d — %s", op, sha, resp.StatusCode, msg)
}
