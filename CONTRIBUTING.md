# Contributing to COGO

Thanks for looking. This is a small project maintained by one person, so the most useful
thing you can send is a clear bug report — but patches are welcome too.

## Getting it running

You need Go 1.25. There is nothing else: no database, no `npm install`, no code generation.

```bash
git clone https://github.com/diegoparras/cogo && cd cogo
go build ./...
go run ./cmd/cogo init
go run ./cmd/cogo serve -http 127.0.0.1:8080 -vault ./vault
```

The repo ships a small demo vault (the `fisherboy-*` notes) so the viewer has something to
show on first run.

## Running the tests

```bash
go test ./...
```

That's the whole suite — 19 packages, no external services, no fixtures to download.

> **If you're on Windows:** some antivirus products quarantine Go's compiled test binaries as
> a false positive (Kaspersky flags them as `Convagent.gen`), which makes `go test` fail in
> confusing ways. If that happens, run the suite in a container instead:
>
> ```bash
> docker run --rm -v "$PWD":/src -w /src -e GOFLAGS=-buildvcs=false golang:1.25 go test ./...
> ```

Before opening a PR, please also run what CI runs:

```bash
gofmt -l .        # must print nothing
go vet ./...
```

## The one rule that isn't negotiable

**Nothing outside the color engine may write a note's confidence.**

The computed block (`confidence`, `color_reason`) is derived from evidence, verification,
freshness and dependencies — never accepted from a caller, an agent, or a model. A patch
that lets anything set a color directly won't be merged, no matter how convenient it is. The
whole product is worthless the moment a note can paint itself green.

Related: `internal/core` stays deterministic and offline. Anything that touches the network —
models, embeddings, GitHub, R2, OIDC — lives outside it and reaches in through an injected
hook (see `SetWriteHook`, `SetArtifactChecker`, `SetGitHubResolver`). Please keep it that way.

## Style

- Match the surrounding code. It's plain Go with the standard library doing most of the work.
- Comments explain **why**, not what. If a line needs a comment to say what it does, the line
  is probably the problem.
- Tests: the point of a test is to fail when the behaviour is wrong. Name what it protects,
  and prefer one test that would catch a real regression over five that restate the code.
- Commit messages: a short subject line, then prose explaining what problem it solves and why
  this way. Look at the existing history for the tone.

## Reporting a bug

Open an issue with what you expected, what happened, and how to reproduce it. Include your
COGO version (`cogo -h` prints it) and how you're running it — Docker, binary, MCP over stdio
or over HTTP. If it's about a note's color, paste the note's frontmatter: the color is a
function of that, so it's usually enough to reproduce.

**For security problems, don't open an issue** — see [SECURITY.md](SECURITY.md).

## Before writing a large feature

Open an issue first and let's talk. COGO deliberately refuses to do plenty of things (it is
not a file browser, not a task manager, not a wiki), and I'd rather tell you that before you
spend a weekend on it than after.

## License

By contributing you agree that your contribution is licensed under the MIT License, same as
the rest of the project.
