---
id: fisherboy-logout-cookie
type: mistake
project: fisherboy
---

## Claim
Clearing the session cookie must repeat the SAME attributes used to set it (path, secure, sameSite, httpOnly) or the browser keeps it — logout silently fails.
