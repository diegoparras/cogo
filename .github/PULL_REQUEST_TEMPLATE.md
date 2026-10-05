<!--
Thanks for the patch. Keep this short — a paragraph of prose beats a filled-in form.
If it's a large feature, please open an issue first (see CONTRIBUTING.md).
-->

## What problem does this solve?

<!-- The situation before your change, not a restatement of the diff. -->

## How did you verify it?

<!--
What you actually ran. `go test ./...` at minimum; if it touches the viewer, say what you
clicked and what you saw. "Should work" is not verification — this project is literally
about the difference.
-->

---

- [ ] `gofmt -l .` prints nothing and `go vet ./...` is clean
- [ ] `go test ./...` passes (on Windows, see the antivirus note in CONTRIBUTING.md)
- [ ] Nothing outside the color engine writes a note's `confidence`
- [ ] If it reaches the network, it stays out of `internal/core` and goes through an
      injected hook, off by default
