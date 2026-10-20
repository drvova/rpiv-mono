# rpiv for PiG (Pi in Go)

The Go port of the `@juicesharp/rpiv-*` family for [PiG](https://github.com/MichaelKinsy/PiG), the Go implementation of [Pi Agent](https://github.com/badlogic/pi-mono). Every published rpiv extension is a PiG Go extension here, composed with the unchanged skills under `packages/rpiv-pi/skills` by one Piglet manifest at the repository root. You run the pipeline with `pig --piglet piglet.yaml`; nothing in this directory needs Node.js.

## Install

PiG builds Go extensions from source the first time it loads them, so you need `pig` and a Go toolchain:

```sh
go install github.com/MichaelKinsy/PiG/cmd/pig@latest   # or see pi-in-go.dev/install
pig setup go                                             # installs a verified Go when none is on PATH
git clone https://github.com/juicesharp/rpiv-mono.git
cd rpiv-mono
pig --piglet piglet.yaml
```

The first start compiles the extensions (under a minute on a laptop); later starts reuse the build. `/reload` rebuilds an edited extension without leaving the session.

## Quick start

```text
/skill:discover
```

The `discover` skill interviews you about what you want to build and writes the first artifact under `.rpiv/artifacts/`. The rest of the pipeline (`research`, `design`, `plan`, `implement`, `validate`, `code-review`, `commit`) chains from there, or runs end to end with `/wf build`.

Check what loaded with `/rpiv-setup`: it lists the sibling extensions the pipeline found and tells you how to select any that are missing.

## What you get

| Extension | Role | Selected by `piglet.yaml` |
| --- | --- | :---: |
| `rpiv-pi` | The pipeline: session hooks, `/rpiv-setup`, `/rpiv-update-agents`, `/rpiv-models`, `/lanes`, built-in `/wf` workflows | ✓ |
| `rpiv-workflow` | `/wf` runner: typed multi-stage workflows with audited JSONL state, predicate routing, parallel fan-out | ✓ |
| `rpiv-agents` | The `Agent` tool: named subagents run as detached `pig --mode rpc` sessions (replaces pi-subagents) | ✓ |
| `rpiv-args` | `$1` / `$ARGUMENTS` placeholders and `` !`cmd` `` substitution in skills | ✓ |
| `rpiv-ask-user-question` | Structured questionnaire overlay the model asks you with | ✓ |
| `rpiv-todo` | Live task overlay that survives `/reload` and compaction | ✓ |
| `rpiv-advisor` | Second opinion from a stronger reviewer model before the agent acts | ✓ |
| `rpiv-web-tools` | `web_search` / `web_fetch` with pluggable providers | ✓ |
| `rpiv-i18n` | Localization SDK, `/languages`, `--locale` | ✓ |
| `rpiv-btw` | `/btw` side question in an ephemeral overlay | opt-in |
| `rpiv-voice` | `/voice` on-device dictation (sherpa-onnx Whisper, cgo) | opt-in |
| `rpiv-warp` | Warp terminal notifications over OSC 777 | opt-in |
| `rpiv-telemetry` | MLflow tracing of lifecycle and subagent activity | opt-in |

Load an opt-in extension for one run with `pig --piglet piglet.yaml -e pig/extensions/rpiv-btw`, or write a Piglet that `extends` this one and adds it.

## Configuration

Config files keep the same paths and JSON shapes as the TypeScript extensions, so a machine that already runs rpiv under Pi needs no migration: `~/.config/rpiv-<name>/` (honoring `XDG_CONFIG_HOME`, mode 0600). Agent-directory state that Pi kept under `~/.pi/agent` lives under `~/.pig/agent` for PiG.

| Setting | What it does | Default |
| --- | --- | --- |
| `PIG_BIN` | The `pig` executable child sessions (workflow stages, subagents) run | `pig` on `PATH` |
| `PIG_PIGLET_PATH` | Piglet the child sessions load, when set in the environment | inherited |
| `~/.config/rpiv-pi/models.json` | Per-skill and per-stage model and effort overrides (`/rpiv-models`) | none |
| `~/.config/rpiv-workflow/` | User workflow overlays and packs | none |

## Reference

- [`PORTING.md`](./PORTING.md): how the port is structured, the fidelity rules, and the testing pattern.
- Each `extensions/<name>/README.md`: install, configuration and the divergences from the TypeScript extension.
- [rpiv-pi.com](https://rpiv-pi.com): the pipeline narrative and the subagent map (host-agnostic).
- [pi-in-go.dev/docs](https://pi-in-go.dev/docs/latest/): PiG itself (Piglets, extensions, RPC mode).

## Requirements

- PiG 0.5 or newer and Go 1.26 or newer (`pig setup go` installs one).
- `node` on `PATH` only for the skills' helper scripts under `packages/rpiv-pi/skills/_shared/` (the extensions themselves do not use it).
- `rpiv-voice` needs cgo and a C compiler for the sherpa-onnx and miniaudio bindings.

## Development

```sh
make -C pig all                  # gofmt, vet, build, test every module (GOWORK=off, as PiG builds them)
make -C pig validate PIG=pig     # pig install --validate-only on every extension
```

## License

MIT, see [LICENSE](../LICENSE).
