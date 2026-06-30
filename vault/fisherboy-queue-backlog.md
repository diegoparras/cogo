---
id: fisherboy-queue-backlog
type: bug
project: fisherboy
evidence:
  - kind: hypothesis
    ref: "guessed from slow responses, not measured"
check:
  test: "inspect the redis queue length and worker consumption rate"
  status: not_run
last_verified: 2026-06-28
---

## Claim
The redis queue is probably backed up because responses got slow.

## Minimal check
Inspect the redis queue length and the worker consumption rate before assuming.
