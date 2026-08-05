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

### The 16 tools your agent gets

| tool | what it does |
|---|---|
| `pack` | colored context on a topic **before acting** — red is quarantined as do-not-rely |
| `authorize` | **ask whether what you know is enough for what you're about to do** |
| `search` | find notes by meaning (embeddings) or by keyword (BM25) |
| `open` | one note, with its freshly computed color |
| `capture` | record a finding — evidence and a check are required, a color is not accepted |
| `verify` | mark the check as passing today and re-color |
| `gap` | **record what the project does NOT know, as an open question** |
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

## Two tools that don't exist anywhere else

### `gap` — modeling what nobody knows

Every memory tool stores what a project knows. None of them stores what it **doesn't**.

Without that, an agent cannot tell a topic nobody investigated from a topic that doesn't
exist. Both look identical: silence.

```yaml
type: gap
question: Does the connection pool saturate under sustained load?
blocks: [migrate-db, scale-replicas]
cost_to_resolve: medium
attempted:
  - checked the dashboard — fine during business hours, never tested under real load
```

A gap **carries no color**, and that's the whole point. Painting it red would be tempting —
there's no evidence, after all — and wrong: a red note *asserts* something unsupported, a gap
asserts nothing. It would turn a good question into a bad claim.

The pack returns them **last, in their own section**, ordered by how many decisions each one
blocks. The tool description ends with the line that matters: *"if you're about to guess,
record the gap instead."*

### `authorize` — the tool that can say no

COGO tells you how much each thing you know is worth. That's half of it. The other half is
that **how much it needs to be worth depends on what for**.

| action class | needs by default |
|---|---|
| informative — answer, explain | `asserted` |
| reversible — edit, create, commit | `check_declared` |
| costly — deploy, migrate, spend | `claimed_passed` |
| **irreversible** — delete, publish, send, force-push | **`verified`** |

That last row is the line: **the only class that demands an *executed* check, not a declared
one.** An agent's word doesn't get you there.

And the class isn't the agent's to choose — because whoever wants to act is exactly who's
incentivized to classify it low. *"I'll clean up some temp files"* can be an `rm -rf`. So the
class is decided **twice** — declared and inferred from the text — and **the stricter wins**.

```
authorize("clean up some temp files with rm -rf in the build folder",
          class: "informative", notes: ["pool-limit-200"])

NOT AUTHORIZED — an irreversible action needs `verified` support, and the note
does not reach it
  · pool-limit-200 is at claimed_passed — the check is declared as passing but
    nobody executed it: run it with the runner

action class: irreversible (declared "informative", but the text says
              irreversible (file deletion): the stricter one wins)
```

## The viewer

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

### God mode — the war room

Every rules engine has constants. Scattered through the code they're invisible: nobody knows
they exist, nobody knows what happens if they move, and changing one means a recompile.

COGO's **21 parameters live in one registry**, each with its label, what it does, its unit,
its valid range and **what it loosens if you move it**. The panel is *generated* from that
registry — there's no hand-written list of controls that can drift from what the engine reads.

Entering asks you to type **acepto**, and the modal says plainly what it implies: that what
you move re-colors notes that already exist without anyone re-verifying them, and that every
change lands in the audit log with your name.

Behind the gate, a second tab shows **what the engine is doing right now** — not knobs, state:

- **chain integrity, first** — the only fact that invalidates every other one on the screen
- **all eight lattice states, not three colors.** The difference shows up immediately:
  *"17 in `claimed_passed`, 0 in `verified`"* means seventeen notes that look green and not one
  with a check anybody actually ran
- the **event log**, the **runner**, every **authorization** an agent asked for, and **graph
  health** — cycles and dangling dependencies, listed separately because they look like the
  same red in the Vault and are fixed differently

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

GitHub references are anchored to the **blob SHA**. Artifacts are keyed by their **SHA-256** —
locally or on Cloudflare R2 — so `verify` **recomputes** the hash instead of trusting a
citation that rots. A **secret scanner runs before anything is stored**, and refuses by default.

### And a changed file doesn't mean a changed claim

A note cites `docker-compose.yml:164`. Someone runs a formatter, or adds a license header, and
the note used to go yellow: the **file** hash changed. Line 164 still said exactly the same
thing.

That's not a harmless false positive. **A warning that fires for anything trains people to
ignore it** — and then the day the file really changes where the note stood, yellow means
nothing.

So the anchor isn't "line 164": it's the **text** that was on line 164, and COGO looks for it
wherever it ended up.

```
still there, same place        →  no change
still there, different line    →  no change — and it tells you where it moved
whitespace only                →  no change
where you cited now says       →  CHANGED
something else
```

Relocating has a trap, and it's handled: a one-line citation that reads `}` matches anywhere.
Finding it wouldn't be finding it, it'd be guessing — and guessing here means **absolving a
real change**. So a region needs enough distinctive text, and it has to appear **exactly once**.

## How the color is computed

```
confidence = meet( check axis , evidence ceiling , freshness ,
                   contradictions , citation materiality , the graph )
```

`meet` is a lattice infimum: commutative, associative, idempotent — with tests for all three.
The practical consequence is that **no axis can raise a color, only lower it**. That makes it a
*must* analysis: it asserts only what holds along every path.

A note is green only when **nothing** drags it down. Every color ships its `color_reason`, so
you can always audit **why**.

### The color is a fold of an append-only log

Since the current version, the color isn't computed from the note's fields — it's computed by
folding a **hash-chained event journal** through a **9-state machine**, then resolving a
**greatest fixed point** over the dependency graph.

- **Bitemporal.** Each event carries when it happened in the world and when COGO recorded it.
- **Tamper-evident.** Each event's hash includes the previous one: altering an old event
  invalidates everything after it, and the war room says so at the top.
- **Multi-process safe.** Writes take an OS lock (`flock` / `LockFileEx`) — a rolling deploy
  briefly runs two containers on one volume, and without it they'd both claim the same sequence
  number and fork the chain.

The state machine is generated from a single YAML table, and the generator **fails the build**
on a malformed one: a state with no rank, duplicated or gapped ranks, a transition to a
nonexistent state, an orphan guard, an unreachable state, two transitions sharing a guard, or
an uncovered decision.

**And only the internal runner can produce `verified`.** That isn't a convention: the emitter
is reserved, the journal *rejects* it through the ordinary door, and a single function emits it
— so one `grep` shows every place in the system capable of producing a verification.

### Five invariants, on hundreds of random vaults

Checked on every run of the suite, over generated vaults with cycles and arbitrary
dependencies:

1. **Determinism** — Go's map iteration order is deliberately random; the result isn't.
2. **Propagation only lowers** — leaning on something can't make you more trustworthy.
3. **A contradiction never improves anything** — or the system would reward hiding them.
4. **Removing evidence never raises** — what's asserted is asserted because there's something
   behind it.
5. **Nobody reaches `verified` without executing.**

> The original plan called for TLA+. A formal model is a *separate artifact*, and what it
> verifies is the model: nothing guarantees the model and the Go say the same thing, and the
> moment they drift the checker certifies a system that isn't the one running. These properties
> are weaker — they cover generated cases, not all of them — and they're true of **the code that
> ships**.

Invariant 3 found a real defect the day it was written: opening a contradiction on an
already-refuted note *raised* it from `refuted` to `contradicted`. Recording a problem improved
the note. Both states are red, so no color test would ever have seen it.

### And COGO forgets

It used to grade everything that came in and never take anything out. Color isolates red but
doesn't remove it — a three-year-old vault is thousands of notes, nearly all expired, **and the
dead cover the living**.

Forgetting by age would have been the obvious mistake: the oldest note in a vault might be the
most consulted one. So a note goes **dormant** only when all of these hold: it expired (twice
its window), nothing depends on it, and nobody consulted it in 180 days. Constraints, pinned
notes, contradicted notes and open questions never go dormant.

**Dormant isn't deleted.** It's still a file, still opens by id, still shows in the viewer with
its reason — it just stops entering the pack. Search still returns it, marked, because search
is how you find it to wake it.

**And it's computed, not written** — like the color. Consult it and it stops being unconsulted,
so it stops being dormant. There's no state anyone has to remember to undo.

### And it records who decided

An agent proposes Fastify. You say "sure". The agent records *"we decided on Fastify"*, with
its author and its evidence. Tomorrow it reads that back as an established project fact. Each
round launders an opinion into a fact.

The other axes can't see it: the evidence can be impeccable — a `file_read` of the
`package.json` the agent itself wrote — and attestation says who ran the check, not who had
the idea.

So normative notes (`decision`, `constraint`) carry an **origin**: `human`, `agent` or
`instrument`. Only those two types, because a `bug` describes how the world is and evidence
answers for it; a decision asserts that somebody **chose**, and no command output can prove a
choice.

In the pack:

```
- origin: **proposed by an agent** — no human chose this; it is open to revision
```

## How it compares

COGO isn't the first tool to store what you know, and not even the first to verify it.
Notion and Confluence let a human mark a page as verified with an expiry date. GitHub's
Copilot Memory pins facts to code citations and re-checks them against the current branch.
Several agent-memory projects carry a `confidence` field.

Being precise about what's actually different:

| | Notes<br>(Obsidian, Notion) | Agent memory<br>(mem0, Zep, Letta…) | Copilot<br>Memory | **COGO** |
|---|:---:|:---:|:---:|:---:|
| Stores what you know | ✅ | ✅ | ✅ | ✅ |
| Marks what's verified | by hand | — | ✅ | ✅ |
| The system decides it, not the model | — | — | ✅ | ✅ |
| **Three levels, not yes/no** | — | — | — | ✅ |
| Re-checks the evidence | — | — | ✅ | ✅ |
| **Doubt propagates through dependencies** | — | — | — | ✅ |
| **Tells the agent what *not* to use** | — | — | — | ✅ |
| **Separates "somebody said" from "a machine ran it"** | — | — | — | ✅ |
| **Stores what the project does NOT know** | — | — | — | ✅ |
| **Can refuse an action for lack of backing** | — | — | — | ✅ |
| **Forgets what nobody uses** | — | — | — | ✅ |

The ones that are actually ours:

- **A color the model is forbidden to write.** Where other tools have a confidence field,
  it's usually the LLM that fills it in — and once written it never changes. COGO computes
  it from the evidence, and recomputes it every time you look.
- **Doubt that propagates.** Everyone else resolves contradictions *pairwise* and stops
  there. A survey of 435 papers on agent memory names this as an open problem: *"supersession
  is local; derived records are not re-examined."* COGO's `meet` over the weakest dependency
  is exactly that missing piece.
- **Quarantine instead of filtering.** Other systems use verification to *hide* the doubtful
  from the agent. COGO hands it over labeled as an assumption, with instructions not to act
  on it. Hiding it means the agent doesn't know what it doesn't know.
- **Declared ≠ executed.** Every tool that "verifies" takes the model's word for it. COGO
  stores *who said so*, and reserves the top of the lattice for checks a machine actually ran.
- **Absence as a first-class object.** `gap` notes make "nobody investigated this" different
  from "this doesn't exist" — two things that look identical everywhere else: silence.
- **Memory that forgets.** Storing is easy. Deciding what stops deserving to be remembered —
  by use and structure, not by age — is the part nobody does.

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
| [Manual](docs/manual.md) | **the full manual** — from "what is this" to the lattice and the fixed point |
| [Parameters](docs/parametros.md) | the 21 knobs behind god mode, one by one |
| [For AI agents](docs/COGO-para-agentes.md) | put this in front of your agent |
| [Autonomy engine](docs/motor-autonomia.md) | Guard, in depth |
| [Veracity engine](docs/motor-veracidad.md) | xray, in depth |
| [Security](docs/seguridad.md) | threat model and hardening |
| [Theory](docs/fundamento-teorico.md) | why the iron rule |

## License

MIT — Diego Parrás, CeMIACE / SEUBES / FCE-UBA. Part of the **Escriba Suite**.
