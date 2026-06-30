---
id: fisherboy-redis-hostname
type: bug
project: fisherboy
evidence:
  - kind: direct_log
    ref: "api log 2026-06-27T14:03Z: connect OK to redis:6379"
  - kind: absence
    ref: "no worker logs since 2026-06-27T13:50Z"
check:
  test: "read the worker's effective env; test internal connectivity to fisherboy-redis:6379"
  status: not_run
last_verified: 2026-06-27
depends_on: [fisherboy-redis-topology]
---

## Claim
The worker likely fails because it can't resolve the internal Redis hostname.

## Refutation
False if the worker has its own REDIS_URL different from the api's.

## Minimal check
Read the worker's effective env and test internal connectivity before editing code.
