# rpiv-mono

[![CI](https://img.shields.io/github/actions/workflow/status/juicesharp/rpiv-mono/ci.yml?branch=main&label=CI)](https://github.com/juicesharp/rpiv-mono/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/juicesharp/rpiv-mono/branch/main/graph/badge.svg?v=2)](https://codecov.io/gh/juicesharp/rpiv-mono)
[![tests](https://img.shields.io/endpoint?url=https://raw.githubusercontent.com/juicesharp/rpiv-mono/badges/tests.json)](https://github.com/juicesharp/rpiv-mono/actions/workflows/ci.yml)

rpiv is a skill-based development workflow for AI coding agents: a pipeline of contract-carrying skills (`discover` → `research` / `design` / `blueprint` → `plan` → `implement` → `validate` → `code-review` → `commit`), the named subagents they dispatch, and the extensions that keep an experienced driver in the loop while the work moves at LLM speed. It now runs natively on [PiG (Pi in Go)](https://github.com/MichaelKinsy/PiG): the extensions are Go, composed by one Piglet manifest, and nothing in the pipeline needs Pi's TypeScript runtime or Node.js. The TypeScript packages that targeted [Pi Agent](https://github.com/badlogic/pi-mono) remain in `packages/` as the legacy implementation and as the single source of the skills and agent profiles both implementations share.

**Where to start:**

- **You want to use it** — install [PiG](https://pi-in-go.dev/install) and a Go toolchain, clone this repo, and run `pig --piglet piglet.yaml`. Install, quick start, and the extension table: [pig/README.md](./pig/README.md). Full narrative and subagent map: [rpiv-pi.com](https://rpiv-pi.com).
- **You want one piece of it** — every extension below stands alone; load one with `pig -e pig/extensions/rpiv-<name>` or select it in your own Piglet. Pick a row from [Packages](#packages).
- **You want to read or hack the code** — [Repo as a repo](#repo-as-a-repo) has the layout, the conventions, and what the gates enforce. The porting contract between the two implementations is [pig/PORTING.md](./pig/PORTING.md).
- **You want to know where this is going** — [roadmap.md](./roadmap.md) for the structured view, [Roadmap](#roadmap) for the reasoning behind it.

## Packages

Every rpiv extension is a Go module under `pig/extensions/`, loaded by PiG from source (PiG builds it on first use and rebuilds on `/reload`). The pipeline needs most of them. **rpiv-args** expands shell-style `$1` and `$ARGUMENTS` placeholders inside skills, **rpiv-ask-user-question** lets the model put a structured questionnaire to the user instead of guessing, **rpiv-todo** keeps a live task overlay that survives `/reload` and compaction, **rpiv-advisor** escalates to a stronger reviewer model before the agent acts, **rpiv-web-tools** gives the model web search and fetch with pluggable providers, **rpiv-workflow** chains skills into typed multi-stage pipelines (audited JSONL state, predicate routing, per-stage output validation) and ships the `/wf` command, and **rpiv-agents** is the `Agent` tool that dispatches the named subagents as detached PiG sessions. A few exist because I wanted them inside the agent: **rpiv-btw** is a side-conversation pattern I got used to in Claude Code, **rpiv-voice** is on-device dictation for when I'd rather talk than type, and **rpiv-warp** integrates with Warp terminal's notification system, because that's where I actually run it. **rpiv-i18n** is the one that came from users: it started as localization for ask-user-question and grew into a small SDK. **rpiv-telemetry** wires the agent into MLflow — auto-instruments lifecycle events and subagent activity so runs are inspectable after the fact. And the libraries are plumbing rather than products: **rpiv-config** is the shared config I/O every sibling depends on (same files and shapes as the TypeScript version, so a machine that ran rpiv under Pi needs no migration), **rpiv-host**, **rpiv-tui**, **rpiv-rpc** and **rpiv-testkit** hold the host view, terminal helpers, child-session client and test harness the extensions are built on.

> [!TIP]
> **For install, quick start, configuration and the development gates of the Go port, see [pig/README.md](./pig/README.md). For the pipeline narrative and the subagent map, visit [rpiv-pi.com](https://rpiv-pi.com).**

### PiG extensions

`piglet.yaml` at the repository root selects the pipeline set; opt-in extensions load with `pig -e pig/extensions/rpiv-<name>` or from a Piglet that `extends` this one.

| Extension | Role | Selected by `piglet.yaml` |
| --- | --- | :---: |
| `rpiv-pi` | Pipeline runtime: session hooks, `/rpiv-setup`, `/rpiv-update-agents`, `/rpiv-models`, `/lanes`, built-in `/wf` workflows | ✓ |
| `rpiv-workflow` | `/wf` runner — chain skills into typed multi-stage pipelines | ✓ |
| `rpiv-agents` | `Agent` tool — named subagents in detached `pig --mode rpc` sessions | ✓ |
| `rpiv-args` | `$1` / `$ARGUMENTS` placeholders and `` !`cmd` `` substitution in skills | ✓ |
| `rpiv-ask-user-question` | Structured questionnaire to the user | ✓ |
| `rpiv-todo` | Live task overlay surviving `/reload` | ✓ |
| `rpiv-advisor` | Escalate to a stronger reviewer model | ✓ |
| `rpiv-web-tools` | Web search + fetch with pluggable providers | ✓ |
| `rpiv-i18n` | Localization SDK for sibling extensions | ✓ |
| `rpiv-btw` | `/btw` side-conversation slash command | opt-in |
| `rpiv-voice` | Local voice dictation (`/voice` overlay, on-device Whisper) | opt-in |
| `rpiv-warp` | Warp terminal notification integration | opt-in |
| `rpiv-telemetry` | MLflow observability — auto-instruments lifecycle + subagent activity | opt-in |

### Everything else in the workspace

| Directory | Role | How you get it |
| --- | --- | --- |
| `pig/extensions/rpiv-config`, `rpiv-host`, `rpiv-tui`, `rpiv-rpc`, `rpiv-testkit` | Shared Go libraries (config I/O, host view, terminal helpers, child-session client, test harness) | Required by the extensions through relative `replace` directives; not loaded by PiG on their own |
| `packages/rpiv-pi/skills`, `packages/rpiv-pi/agents` | The skills and agent profiles (Markdown) | Selected by `piglet.yaml` and embedded by `rpiv-pi`; shared with the legacy implementation |
| `packages/rpiv-*` | The legacy TypeScript extensions for [Pi Agent](https://github.com/badlogic/pi-mono), published to npm as `@juicesharp/rpiv-*` | `pi install npm:@juicesharp/rpiv-pi` for a Pi user; not needed to run the pipeline on PiG |
| [`packages/rpiv-site`](https://github.com/juicesharp/rpiv-mono/tree/main/packages/rpiv-site) | The [rpiv-pi.com](https://rpiv-pi.com) site (static Astro build) | **Not published.** Built from this repo with `npm run build:site`, deployed to GitHub Pages by CI |

## Roadmap

> [!NOTE]
> For the structured, directional view — what's done, what's next, and what's possible — see the companion [**roadmap.md**](./roadmap.md). This section keeps the philosophy.

The realization shaping AI-assisted development right now is that LLMs produce correct code, not aligned code. The output compiles and passes tests, but it isn't enterprise-grade in the way human engineers produce: fitting the codebase's existing patterns, respecting conventions that aren't written down anywhere, making the boring choices mature systems rely on, staying reviewable and extensible by the next person who touches it. Closing that gap takes a driver: an experienced engineer who carries the context the model can't have, who frames the task correctly upfront, who steers architecture, who pushes back when output drifts. Without that active driver, the codebase fills with locally-correct, globally-misaligned diffs that look fine in PR review and erode the team's confidence over time.

Misaligned code isn't zero-value, it's negative-value: it compiles, it ships, then it taxes every engineer who reads that file afterward and costs the next refactor a half-day of reasoning about near-duplicates that shouldn't exist. Worse, it quietly subtracts from the architecture's coherence in a way no PR review catches. Misaligned-diff throughput is alignment-debt throughput. The cost of a driver-in-the-loop pipeline is latency, paid up front and visible on a dashboard; the cost of skipping it is alignment debt, paid later, by someone else, and rarely traced back to the diff that caused it.

rpiv-pi exists to keep the driver meaningfully in the loop while the work moves at LLM speed. The pipeline asks the right questions at the right moments (ask-user-question), surfaces architectural decisions where they matter, and structures human involvement as participation rather than approval. The bet isn't that models will plateau. They'll keep getting better. The bet is that the structural conditions for fully autonomous coding (institutional knowledge availability, connector maturity, correctness and latency at enterprise scale) are a decade-scale problem absent a real breakthrough. For that whole interval, the realistic operating model is a driver in the loop. rpiv-pi is the engine for that loop: it writes and verifies code under expert supervision. A real enterprise harness, the layer that connects an engine like this to ticketing, CI, and an organization's institutional context, is a separate piece of work and not what this project is today. At Codemasters, we're about to start exploring how an engine like rpiv-pi could fit as part of a broader multi-chain experiment. Whether any of that surfaces back here is open.

In parallel, the pipeline is already viable on affordable open-weight models today (GLM-5.1, Kimi K2.5, MiMo-V2-Pro), and produces genuinely good results in practice. Output isn't yet at parity with frontier on the same task: more mistakes than I'd want, longer runs than I'd want. Closing that residual gap is the next set of work below. I'm assessing this as a CTO leading AI adoption at a mid-size company with no frontier-lab budget, where the cost difference matters: as of May 2026, GLM-5.1 lists at roughly a third of Claude Sonnet 4.6's output token price, and around a fifth of Opus 4.7's.

- **Verification under affordable-model runs.** The failure mode I see today on GLM/Kimi/MiMo isn't loud breakage; it's self-validation blindness, and it has two roots, not one. Same model is the obvious one: affordable-model work passes affordable-model verifiers, and only escalation to a frontier judge (currently Opus 4.7, May 2026) reliably catches the residual issues. The less obvious one is same context: a verifier that inherits the author's chat anchors on the same framings, ratifies instead of attacks, and writes tests that encode the author's mental model rather than probe it. Frontier escalation defeats the cost argument; isolation doesn't. The next round of work is experimenting with verification setups that lean on fresh-context isolation first and frontier escalation only where it earns its keep.
- **Delegation strategy.** The pipeline's runtime cost is a function of how work is delegated across skills and subagents: what runs in parallel, what serially, which model handles which step, where verification fires. Some of the slowness on affordable-model runs is infrastructure-side and outside scope. The part that's tractable is finding the delegation pattern that minimizes total run cost without sacrificing output quality. This is an open optimization question, not a planned feature; the next round of work is experimenting with combinations and measuring what actually trades off against what.

## Who built this

Sergii Guslystyi. Software Architect and CTO at Codemasters International. Two decades building software, currently leading AI-first development adoption inside the company. rpiv-mono is a personal initiative, on my own time, where I prototype the patterns I'm working through professionally and put them in public for anyone to use, fork, or push back on.

Find me on X: [@juicesharp](https://x.com/juicesharp).

## Repo as a repo

Two implementations side by side, one set of skills. The Go port lives under `pig/`; the legacy TypeScript packages under `packages/`.

**The Go port (`pig/`).** One Go module per extension under `pig/extensions/`, each exposing PiG's conventional factory `func Extension() *sdk.Extension`. PiG builds every extension with `GOWORK=off`, so each module is self-contained: it requires the PiG extension SDK and reaches sibling libraries through relative `replace` directives. `pig/go.work` exists for editors and workspace-wide commands only. `make -C pig all` runs gofmt, vet, build and `go test -race` per module; `make -C pig validate PIG=pig` runs `pig install --validate-only` on every extension. Assets both implementations need (prompts, locales, agent profiles) are copied into the Go modules and embedded; a lockstep test asserts each copy still matches its original under `packages/`. [pig/PORTING.md](./pig/PORTING.md) is the contract the port follows: identifiers, tool names, schemas, config files, persisted shapes and user-visible strings mirror the TypeScript originals, and every divergence forced by PiG's subprocess model is recorded in the module's `doc.go`.

**The legacy TypeScript packages (`packages/`).** npm workspaces monorepo. Clone, `npm install` at the root, that's it. Node 22+ and npm 11+ (the `engines` floor). A few choices worth naming up front:

- No build step. Packages publish raw `.ts`; Pi loads TypeScript directly. No `dist/`, no per-package tsconfig.
- One Vitest runner at the root walks every package. No per-package vitest configs.
- Lockstep versions. All fifteen workspace packages share one version, enforced by `scripts/sync-versions.js` — published or not.
- Releases are local-only by design. `node scripts/release.mjs <patch|minor|major|x.y.z>` cuts a release; no CI publish workflow.
- Husky gates the work. `pre-commit` runs Biome and `tsc --noEmit` (fast); `pre-push` runs the full test suite with coverage thresholds. Tests don't block commits, they block pushes.
- Single shared config across the workspace: one `biome.json`, one `tsconfig.base.json`, one `vitest.config.ts`.
- Every README follows one shape — see [docs/readme-standard.md](./docs/readme-standard.md) before rewriting one.

### Contributions

Issues are welcome. PRs too, but please open an issue first if the change isn't trivial. The project has a definite shape and direction, and a quick conversation up front saves both of us a round-trip.

## Status and expectations

Started April 2026 as Pi extensions; ported to Go for PiG in October 2026. MIT licensed.

Single maintainer (me); Claude Code is a co-author on most commits.

Actively maintained as a personal project. Issues triaged on best effort. Cadence may slow when the day job runs hot.

## Pointers

- Site: [rpiv-pi.com](https://rpiv-pi.com)
- Go port: [pig/README.md](./pig/README.md) · PiG: [pi-in-go.dev](https://pi-in-go.dev)
- Roadmap: [roadmap.md](./roadmap.md)
- README standard: [docs/readme-standard.md](./docs/readme-standard.md)
- License: [MIT](./LICENSE)
- X: [@juicesharp](https://x.com/juicesharp)
