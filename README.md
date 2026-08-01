<h1 align="center">COGO</h1>

<p align="center">
  <b>Memory with a confidence traffic light — for you and your AI agents.<br>
  Plus the guard that x-rays what a model is telling you.</b>
</p>

<p align="center">
  Every fact you know about your project, colored by how much you can trust it.<br>
  Every turn a model takes, measured by how hard it's pushing you.
</p>

<p align="center">
  <a href="https://github.com/diegoparras/cogo/actions/workflows/docker.yml"><img alt="CI" src="https://github.com/diegoparras/cogo/actions/workflows/docker.yml/badge.svg"></a>
  <img alt="Go 1.25" src="https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white">
  <img alt="MIT" src="https://img.shields.io/badge/license-MIT-black">
  <img alt="MCP" src="https://img.shields.io/badge/MCP-stdio%20%2B%20HTTP-6E56CF">
  <img alt="cosign signed" src="https://img.shields.io/badge/image-cosign%20signed-2ea44f">
</p>

<p align="center"><sub><b><a href="README.es.md">🇦🇷 Leer en español</a></b> · part of the <b>Escriba Suite</b></sub></p>

---

## The problem

When you build software — you, or an agent like Claude Code, Cursor or Copilot — you
accumulate "truths": *the database is on that host*, *the bug is caused by X*, *we decided Y*.

Over time they **rot**. Some were never verified. Some went stale. Some were a hunch someone
typed at 2am. And here's the part that actually hurts:

> **They all look equally true.**

So you act on a guess believing it's a fact. Worse: your agent does — confidently, and at
machine speed.

## What COGO does

COGO stores that knowledge as **plain Markdown notes** and gives each one a **confidence color
that COGO computes itself**:

| | | |
|---|---|---|
| 🟢 | **green** | verified — rely on it |
| 🟡 | **yellow** | probable — not confirmed yet |
| 🔴 | **red** | assumption — do not rely on it |

**You never pick the color.** COGO derives it from four things: is there **evidence**? was it
**checked**? is it **fresh** (facts expire)? does it **depend** on something shaky? That's why
it can't be gamed — nobody gets to paint a note green because they feel good about it.

It lives in a computed block that agents are forbidden to write:

```yaml
# ---- computed by COGO · do not edit ----
confidence: red
color_reason: no observed or reported evidence
```

## What it looks like in practice

You're debugging:

1. You jot down *"the worker can't reach Redis"* — a hunch, no evidence → 🔴 **red**
2. You check the logs, find the proof, attach it as evidence → 🟡 **yellow** (there's evidence,
   but the test that would confirm it hasn't run)
3. You run the test, it passes, you hit **verify** → 🟢 **green**
4. Next week you ask Claude for help. Claude reads your notes, sees Redis is **green** (uses it
   as fact) and something else is **red** (won't build on it). It doesn't waste a turn
   re-investigating what you already proved, and it doesn't act on your hunch.

That's COGO: **a memory with a confidence traffic light, for you and for your tools.**

## Guard — the anti-manipulation x-ray

The other half of COGO. When you talk to an LLM you have no way to tell whether that very
confident answer is real reasoning or **fluent nonsense** — or to notice when a conversation is
walking you, step by step, toward something you never agreed to. That has a name: the
**jailbreak on the human**.

Guard reads each model turn **with the adversary's playbook in hand**: an ontology of **108
manipulation techniques** distilled from the six disciplines that studied how to move a person
against their will — persuasion (Cialdini, Kahneman), **police and military interrogation**
(Reid, Army FM 2-22.3, Scharff), negotiation (Harvard, Voss), **coercion and thought reform**
(Lifton, Biderman), emotional manipulation (gaslighting, DARVO, FOG) and rhetoric/propaganda
(Frankfurt, Grice, Walton). Each technique carries its real source, what it looks like *in a
chat*, and its countermeasure.

It runs **deterministically and offline** by default — no model, no API key, nothing leaves
your machine. Plug a model in and it goes deeper.

**Veracity (`xray`)** is Guard's twin. Instead of *how hard is this pushing me*, it measures
**how much of this answer is actually held up**: the gap between what a text asserts and what
it supports.

## Quickstart

```bash
docker run -d -p 127.0.0.1:8080:8080 -v cogo-vault:/vault -e COGO_ALLOW_INSECURE=1 ghcr.io/diegoparras/cogo
```

Open <http://localhost:8080>. That's it — the viewer ships **inside** the binary. No database,
no build step, nothing else to install.

> `COGO_ALLOW_INSECURE=1` is fine here because the port is bound to your machine. On a server,
> set `COGO_MCP_TOKEN` instead — see [the deploy guide](docs/deploy.md).

<details>
<summary><b>Without Docker</b> — one static binary</summary>

```bash
go install github.com/diegoparras/cogo/cmd/cogo@latest
cogo init && cogo serve -http 127.0.0.1:8080 -vault ./vault
```
</details>

## Connect it to your agent

COGO speaks **MCP** over stdio (local) and Streamable HTTP (remote), so any MCP client works —
Claude Code, Codex, Copilot, OpenCode, Antigravity:

```json
{
  "mcpServers": {
    "cogo": { "command": "cogo", "args": ["serve", "-vault", "./vault"] }
  }
}
```

What Claude learns today, Cursor reads tomorrow: **the same vault.**

### The 14 tools your agent gets

| tool | what it does |
|---|---|
| `pack` | colored context on a topic **before acting** — red is quarantined as do-not-rely |
| `search` | find notes by meaning (embeddings) or by keyword (BM25) |
| `open` | one note, with its freshly computed color |
| `capture` | record a finding — evidence and a check are required, a color is not accepted |
| `verify` | mark the check as passing today and re-color |
| `archive` · `restore` | pull a note out of the graph without destroying it, and bring it back |
| `remove` | actually delete — only for genuine garbage; leaves a tombstone |
| `stash` | store an artifact by content hash → cite it as `artifact://<sha256>` |
| `recall` | re-anchor after a context compaction, or catch up on another agent's work |
| `reflect` | hand in what you did; COGO proposes graded notes worth keeping |
| `lease` | take a TTL'd lease on a resource before a migration, a deploy or a bulk edit |
| `guard` | x-ray a model turn for manipulation pressure |
| `xray` | x-ray an answer for veracity |

> **Shared memory across machines.** Over HTTP + token, any agent on any machine reads and
> writes the same vault. `recall` is the cursor that turns it from an archive into a channel:
> pass back the cursor it gave you and you get **only what changed** since — plus a new cursor.

## The viewer

Seven panels, embedded in the binary:

- **Vault** — a real index: BM25 search, searchable filters, creation-date ranges, pagination.
  Each note shows when it was born, when it was last verified and when it goes stale.
- **Vigencia** *(currency)* — what expired or is about to. Facts have a shelf life.
- **Pack** — the colored context bundle, exactly as an agent receives it.
- **Graph** — how your knowledge connects, and how red propagates down dependencies.
- **Review** — broken links, stale notes and (with a model) contradictions between notes.
- **Guard** · **Veracity** — the two engines, with their evidence.

Plus a Markdown editor that **recomputes the color live as you type**, a **GitHub explorer**
with a confidence map over your repo, an agent-instruction manager (`AGENTS.md`, `CLAUDE.md`…),
a downloadable and prunable audit log, multi-token management, and one-click vault export.

## Evidence that can actually be re-checked

Evidence isn't a vibe, it's a reference COGO can go verify again:

```yaml
evidence:
  - kind: file_read                                  # 9 kinds, from test_result
    ref: worker.go:12                                #  down to hypothesis and absence
  - kind: command_output
    ref: github://acme/api@main/internal/db.go:88    # pinned to the blob SHA
  - kind: direct_log
    ref: artifact://9f2a…                            # content-addressed, immutable
```

GitHub references are anchored to the **blob SHA**, so when the file changes upstream COGO sees
the drift and the note drops to yellow until you re-verify. Artifacts are keyed by their
**SHA-256** — locally or on Cloudflare R2 — so `verify` **recomputes** the hash instead of
trusting a citation that rots. A **secret scanner runs before anything is stored**, and refuses
by default.

## How the color is computed

```
confidence = min( evidence , freshness , weakest dependency , contradiction )
```

A note is green only when **nothing** drags it down. Evidence sets the ceiling: observed (a
log, a command, a test, a file) can reach green; reported or inferred caps at yellow; none is
red. Freshness decays by type — a command lasts 30 days, an architecture decision 180. Every
color ships its `color_reason`, so you can always audit **why**.

## Built to be trusted

- **One static binary.** Go 1.25, `CGO_ENABLED=0`, `scratch` image (~12 MB), assets embedded.
  No database, no runtime, no `node_modules`.
- **Offline by default.** Every network-touching feature — models, embeddings, OIDC, R2,
  GitHub — is an opt-in accessory. The core never phones home.
- **Verifiable supply chain.** Images are signed with **cosign** (keyless Sigstore) and ship
  **SLSA build provenance**. Tests, `go vet` and `gofmt` gate every publish.
- **Your data stays yours.** Notes are Markdown files in a folder you own. Delete COGO and you
  still have everything — readable in any text editor, versionable in git.

## Philosophy

> Not knowing is not a lesser kind of knowing. It's a different thing, and it has to be
> visible.

COGO's iron rule is that **the system never claims more than it can hold up**. The color isn't
a label somebody applies; it's a consequence of the evidence. That's the whole reason it's
worth anything — to you, and against a model that would otherwise be delighted to tell you
exactly what you want to hear.

## Documentation

| | |
|---|---|
| [Installation](docs/instalacion.md) | get it running, step by step |
| [Deploy](docs/deploy.md) | your machine, a server, or a whole team |
| [Manual](docs/manual.md) | how to actually use it |
| [For AI agents](docs/COGO-para-agentes.md) | put this in front of your agent |
| [Autonomy engine](docs/motor-autonomia.md) | Guard, in depth |
| [Veracity engine](docs/motor-veracidad.md) | xray, in depth |
| [Security](docs/seguridad.md) | threat model and hardening |
| [Theory](docs/fundamento-teorico.md) | why the iron rule |

## License

MIT — Diego Parrás, CeMIACE / SEUBES / FCE-UBA. Part of the **Escriba Suite**.
