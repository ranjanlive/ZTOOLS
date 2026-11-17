# Native JSON tools for ZAgent

Each platform folder now contains the binary, its JSON tool manifest, and config.json together. **No Node.js, Python, shell wrapper, or separate adapter is required at runtime.** JSON handling is compiled into every Go binary.

## Folder layout

Windows example:

```text
web_search/
  web_search.exe
  web_search.json
  config.json
```

Linux/macOS use the same layout with `web_search` instead of `web_search.exe`.

Each of the five tools has this layout under `<tool>/binaries/<platform>/`. All thirty platform folders include their matching manifest. The `agent_tools/<tool>/<tool>.json` files are canonical manifest templates; use the ready-to-copy files beside the platform binaries for installation.

## Install

1. Choose the OS/architecture folder for your system, such as `web_search/binaries/windows-amd64` or `web_search/binaries/linux-arm64`.
2. Copy its three files into `<ZAGENT_HOME>/tools/web_search/`. Repeat for the other tools you want. The folder must match the manifest name, as required by ZAgent.
3. Put the extension popup's Connection ID in `config.json`. Reload the extension after an update. For image/music generation, sign in to the relevant browser account.
4. On Linux/macOS, mark each binary executable with `chmod +x`.
5. Check with `zagent tools list`, then run a tool:

```sh
zagent tools run web_search --args '{"query":"Go tutorials","result_count":5}'
zagent tools run webpage_read --args '{"url":"https://example.com"}'
zagent tools run zagent_in_chrome --args '{"action":"navigate","url":"https://example.com"}'
```

The optional setup scripts also install the matching manifest beside each binary under `tools/<tool>/`.

## Relative binary path

Every manifest directly launches its neighbor:

```json
"command": ["{dir}/web_search", "--agent-json"],
"input": "stdin_json",
"output": "json"
```

ZAgent expands `{dir}` to the manifest folder. Its Go process launcher resolves the `.exe` suffix on Windows; Linux/macOS launch the extensionless file. The same JSON therefore works across these operating systems. Choose the correct architecture's binary once, or let the setup script choose it. Moving the three files together requires no binary-path edit.

## Supported arguments

Every current CLI argument remains supported in `parameters.properties`, with descriptions, types, limits, and required fields:

| Tool | Required argument | Supported arguments |
| --- | --- | --- |
| `generate_image` | `prompt` | 28 |
| `generate_music` | `prompt` | 27 |
| `web_search` | `query` | 24 |
| `webpage_read` | `url` | 26 |
| `zagent_in_chrome` | `action` | 49 |

Use underscore keys in JSON. Arrays are supported for `reference_image`, `file`, `coordinate`, `end_coordinate`, and `region`; false and zero values are preserved. Unknown keys are rejected. Use `result_count` or its alias `max_results`, not both.

Inspection flags remain available: `help`/`h`, `version`, `print_config`, and `init_config`. The agent schema still requires the primary argument, even when inspection ignores it: `{"query":"","version":true}` or `{"action":"tabs","help":true}`. Printed Connection IDs are redacted from agent results and diagnostics.

## JSON protocol and outputs

The agent invokes `--agent-json`, sends one JSON object on stdin, and receives one ZAgent result object on stdout:

```json
{"output":"result text or structured CLI JSON","is_error":false,"media":[{"path":"/absolute/path/image.png","mime":"image/png"}]}
```

The binary translates arguments internally and calls its existing CLI implementation in the same process. The ordinary CLI flags and exit codes remain available when `--agent-json` is not used.

Image/music files and browser screenshots/downloads/page saves default to `ZAGENT_OUTPUT_DIR` when no explicit destination is given. Explicit `path`/`output` values win. Page-reading and search results stay inline by default. Generated media paths are returned for attachment, and progress is sent to stderr and `ZAGENT_NOTIFY_FILE`. Errors use `is_error:true`.

Image/music manifests use engine background tasks; other tools run in the foreground. The optional argument `background` separately controls browser-tab focus. The extension still queues same-tool requests and permits different tools to run in parallel.

The known intermittent `NAVIGATION_ABORTED` issue in RELEASE-NOTES.md remains separate from this packaging/protocol change.

## Development

The native adapter is `internal/browsercli/agent.go` in each self-contained source module; maintain the canonical copy under `web_search/source_code`. Go tests cover JSON input, argument conversion, generated files, progress notes, errors, and redaction.

Manifest generation and optional developer checks use Node, but generated tools do not:

```sh
node scripts/generate-agent-tools.cjs
node --test scripts/agent-tools.test.cjs
```

The generator writes each canonical manifest, all six platform copies, and a manifest beside each source module for local builds. `scripts/build.ps1` rebuilds the Go binaries and keeps platform manifests/configs together.
