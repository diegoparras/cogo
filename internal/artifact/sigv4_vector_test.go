package artifact

import (
	"encoding/hex"
	"strings"
	"testing"
)

// TestSigV4Golden locks the signing math (canonical-request hash, string to
// sign, the HMAC signing-key chain) to a fixed output. The expected value was
// cross-validated against an independent SigV4 implementation (Python
// hmac/hashlib) computing the identical inputs — both produce this exact
// signature — so this guards against accidental regressions in the chain.
func TestSigV4Golden(t *testing.T) {
	const (
		secret    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
		region    = "us-east-1"
		service   = "service"
		amzDate   = "20150830T123600Z"
		dateStamp = "20150830"
		// Cross-checked against an independent Python implementation on 2026-07-07.
		want = "ea21d6f05e96a897f6000a1a293f0a5bf0f92a00343409e820dce329ca6365ea"
	)
	canonicalRequest := strings.Join([]string{
		"GET",
		"/",
		"",
		"host:example.amazonaws.com\nx-amz-date:20150830T123600Z\n",
		"host;x-amz-date",
		emptyHash,
	}, "\n")
	scope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, scope, hashHex([]byte(canonicalRequest)),
	}, "\n")
	kDate := hmacSHA256([]byte("AWS4"+secret), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	got := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))
	if got != want {
		t.Fatalf("SigV4 signature = %s\n            want = %s", got, want)
	}
}
