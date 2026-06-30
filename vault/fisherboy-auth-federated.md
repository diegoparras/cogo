---
id: fisherboy-auth-federated
type: decision
project: fisherboy
evidence:
  - kind: file_read
    ref: "docker-compose.suite.yml:160-166 — AUTH_MODE: federado; LOCKATUS_REDIRECT_URI .../auth/callback"
check:
  test: "confirm AUTH_MODE=federado and the Lockatus client id + redirect in the compose"
  status: passed
last_verified: 2026-06-24
---

## Claim
Fisherboy authenticates via Lockatus OIDC (federated): client id `fisherboy`, callback at `/auth/callback`.

## Minimal check
Confirm AUTH_MODE and the LOCKATUS_* envs in the suite compose.
