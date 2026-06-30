---
id: fisherboy-redis-topology
type: architecture
project: fisherboy
evidence:
  - kind: file_read
    ref: "docker-compose.suite.yml:164 — REDIS_URL: redis://fisherboy-redis:6379/0"
check:
  test: "grep REDIS_URL in the suite compose and confirm the redis service name"
  status: passed
last_verified: 2026-06-20
---

## Claim
Fisherboy reaches Redis at the internal hostname `fisherboy-redis`, port 6379, database 0.

## Minimal check
Read REDIS_URL in docker-compose.suite.yml and confirm the redis service name matches.
