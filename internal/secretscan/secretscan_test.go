package secretscan

import (
	"strings"
	"testing"
)

func TestScanCatchesKnownShapes(t *testing.T) {
	cases := map[string]string{
		"AWS access key id":     "deploy log: AWS_KEY=AKIAIOSFODNN7EXAMPLE done",
		"OpenAI/OpenRouter key": "using sk-or-v1-abcdef0123456789abcdef0123456789 for calls",
		"Cloudflare API token":  "token value cfut_ExampleFakeToken0123456789abcd",
		"private key":           "-----BEGIN OPENSSH PRIVATE KEY-----\nb3Blbn...",
		"credentials in URL":    "DATABASE_URL=postgres://user:s3cr3tPass@db:5432/app",
		"secret assignment":     `config: api_key = "9f8e7d6c5b4a3f2e1d0c9b8a"`,
	}
	for wantRule, content := range cases {
		f := Scan([]byte(content))
		if len(f) == 0 {
			t.Errorf("%s: no finding for %q", wantRule, content)
			continue
		}
		hit := false
		for _, x := range f {
			if x.Rule == wantRule {
				hit = true
				if !strings.Contains(x.Preview, "•") {
					t.Errorf("%s: preview %q is not masked", wantRule, x.Preview)
				}
			}
		}
		if !hit {
			t.Errorf("%s: findings %+v did not include the expected rule", wantRule, f)
		}
	}
}

func TestScanIgnoresCleanAndPlaceholders(t *testing.T) {
	clean := []string{
		"the worker retried 3 times then failed with exit 1",
		"password: changeme",
		"token: <your-token-here>",
		"api_key: xxxxxxxxxxxxxxxx",
		"secret = 0000000000000000",
		"just some prose about tokens and passwords in general",
	}
	for _, c := range clean {
		if f := Scan([]byte(c)); len(f) != 0 {
			t.Errorf("false positive on %q: %+v", c, f)
		}
	}
}

func TestGuardPolicy(t *testing.T) {
	secret := []byte("AWS_KEY=AKIAIOSFODNN7EXAMPLE")

	// clean → not blocked, unchanged
	if out, f, blocked := Guard([]byte("all good"), false); blocked || len(f) != 0 || string(out) != "all good" {
		t.Fatalf("clean guard: out=%q f=%+v blocked=%v", out, f, blocked)
	}

	// secret + redact=false → BLOCKED, content unchanged
	out, f, blocked := Guard(secret, false)
	if !blocked || len(f) == 0 || string(out) != string(secret) {
		t.Fatalf("refuse: blocked=%v f=%+v", blocked, f)
	}

	// secret + redact=true → not blocked, secret gone from content
	out, f, blocked = Guard(secret, true)
	if blocked || len(f) == 0 {
		t.Fatalf("redact: blocked=%v f=%+v", blocked, f)
	}
	if strings.Contains(string(out), "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("redacted output still contains the secret: %q", out)
	}
	if !strings.Contains(string(out), "[REDACTED:") {
		t.Fatalf("redacted output missing marker: %q", out)
	}
}
