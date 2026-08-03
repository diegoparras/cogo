# Security Policy

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's private reporting: **[Security → Report a vulnerability](https://github.com/diegoparras/cogo/security/advisories/new)**.
It creates a private thread visible only to you and the maintainer.

If that doesn't work for you, email **diegoparras@gmail.com** with `COGO SECURITY` in the
subject line.

Please include: what you found, how to reproduce it, and what an attacker gains. A proof of
concept helps a lot. You'll get a first reply within 72 hours, and I'll tell you honestly
whether I consider it a vulnerability and when I expect to fix it.

This is a project maintained by one person. There's no bug bounty, but you will get credit
in the advisory unless you'd rather stay anonymous.

## What COGO assumes about its environment

Knowing the threat model saves you from reporting things that are working as designed:

- **Whoever reaches the port reaches the vault.** The MCP server over HTTP refuses to start
  without `COGO_MCP_TOKEN` unless you explicitly set `COGO_ALLOW_INSECURE=1`, which is only
  meant for a port bound to `127.0.0.1`. Running it insecure on a public interface is a
  configuration mistake, not a vulnerability — but *bypassing* that refusal is.
- **Tokens are bearer tokens.** Anyone holding one can do whatever that token's scope
  allows. They are stored hashed; issue one per application and revoke individually.
- **The core is offline.** Models, embeddings, OIDC, Cloudflare R2 and GitHub are opt-in
  accessories. If the core ever reaches the network without you enabling anything, that's a
  bug worth reporting.
- **Your notes are plain files.** COGO does not encrypt the vault at rest. Filesystem
  permissions are the boundary. Disk encryption is your operating system's job.
- **The check runner executes commands, and it is off unless you turn it on.** When enabled,
  it runs *only* commands you declared yourself in `.cogo/runner.yaml` — argv lists, not
  shell lines, each with its own working directory and timeout. A note cannot bring its own
  command; it can only name one you already authorised.

  This is deliberate and it is the whole design. An allowlist of *programs* would not help:
  if an agent can write both the note and the command, and `go test` is allowed, then the
  agent gets `go test` to run code the agent itself wrote — `TestMain` and `init()` execute
  arbitrary code, and `npm test` runs whatever `package.json` says. The line that matters is
  not which binary runs, it is **who decides what runs**.

  The executed command does not inherit COGO's environment (your MCP token and model API key
  do not travel to it); only variables you list explicitly are passed. Output is truncated.
  Timeouts are enforced, and a timeout is reported as *unobserved*, never as a failed check.

  Known limitation, stated plainly: a command that spawns children can leave orphans behind
  after its timeout. COGO stops waiting for them, but does not kill the process tree —
  doing that portably means process-group handling that differs per operating system.

## What I consider a vulnerability

- Reading or writing the vault without a valid token, or across token scopes.
- Escaping the vault directory through a crafted note id, path or evidence reference.
- Leaking the model API key, a token's secret, or vault contents through any response,
  including error messages and the export zip.
- The secret scanner failing to stop a credential from being stored in an artifact.
- Anything that lets a captured note write its own `confidence` field. The computed block is
  the whole premise of the product: if an agent can paint itself green, COGO is worthless.
- Anything that gets a command executed which is **not** one declared in `.cogo/runner.yaml`,
  or that runs one with a different working directory or environment than declared.
- Anything that produces a `verified` state without the runner actually executing a check —
  for example forging the reserved emitter in the journal.
- Remote code execution, SSRF through evidence resolution, or command injection.

## Supported versions

Fixes go to `main` and to the next tagged release. There are no long-term support branches;
please run a recent version.
