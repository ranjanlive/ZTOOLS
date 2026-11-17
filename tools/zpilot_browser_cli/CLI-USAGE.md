# Standalone browser CLI tools

Five independent executables connect through the Connection ID copied from the ZPilot Browser popup. No native host or browser registration is required. See [README.md](README.md) for setup.

| Executable | Required flag | Purpose |
| --- | --- | --- |
| `generate_image` | `--prompt` | Generate or edit an image through ChatGPT |
| `generate_music` | `--prompt` | Generate music through Gemini |
| `web_search` | `--query` | Search using a browser search engine |
| `webpage_read` | `--url` | Read a rendered page as Markdown, text, or HTML |
| `zagent_in_chrome` | `--action` | Navigate, read, click, type, upload, download, and capture pages |

Each binary contains its connection bridge, browser launcher, CLI, sessions, and file output. The first command starts its embedded bridge automatically; it exits after 60 idle seconds. No separate runtime dependency beyond the extension and browser is required. Six OS/architecture targets are in each tool's `binaries` directory.

Each platform folder also contains `<tool>.json` beside its binary and `config.json`. The manifest launches `{dir}/<tool> --agent-json`, which reads a JSON arguments object from stdin and returns ZAgent's JSON result envelope. Windows resolves the adjacent `.exe`; Unix uses the extensionless binary. No Node.js, Python, or wrapper process is required. Copy these three files into the agent's `tools/<tool>/` folder. See [agent_tools/README.md](agent_tools/README.md) for installation and the protocol.

Build from a tool's `source_code` directory with `go build -trimpath -o TOOL_NAME .` (add `.exe` on Windows). PowerShell users can rebuild everything with `./scripts/build.ps1`; `-CurrentPlatformOnly` limits it to this machine. Go is needed only to build.

If a flag is unknown, inspect `Get-Command web_search -All` on Windows or `command -v web_search` on Unix. Select the new executable explicitly if an old version is earlier on PATH. PowerShell needs `./` or `.\` for a binary in the current directory.

## Examples

Commands below assume the tool folders are on `PATH`. Windows also accepts the `.exe` suffix.

```powershell
generate_image --prompt "A watercolor fox"
generate_image --prompt "A watercolor fox" --name fox.png --path "C:\Art"
generate_image --prompt "Make the fox jump" --session fox-story --chat_id "abc123"
generate_image --prompt "A fox in winter" --session fox-story --style "Soft watercolor, muted colors" --chat_limit 12
generate_image --prompt "Change the background" --reference_image "C:\Art\fox.png"
generate_image --prompt "A wide landscape" --aspect 16:9 --output "C:\Art\landscape.webp"

generate_music --prompt "Calm piano intro" --name intro --path "C:\Music"
generate_music --prompt "A matching outro" --session podcast --chat_id "abc123" --vocals instrumental --length short
generate_music --prompt "Lo-fi study music" --cover=false

web_search --query "Go concurrency patterns"
web_search --query "Go concurrency patterns" --result_count 5 --engine bing --json
web_search --query "browser extensions" --site developer.chrome.com --path results.json

webpage_read --url "https://example.com"
webpage_read --url "https://example.com" --path page.md
webpage_read --url "https://example.com" --format html --path page.html
webpage_read --url "https://example.com" --offset 50000 --max_chars 10000

zagent_in_chrome --action navigate --url "https://example.com"
zagent_in_chrome --action read_page
zagent_in_chrome --action click --ref "REF_FROM_READ_PAGE"
zagent_in_chrome --action type --text "Hello" --submit
zagent_in_chrome --action screenshot --path screenshot.jpg
zagent_in_chrome --action upload --file "C:\Documents\report.pdf" --ref "UPLOAD_REF"
zagent_in_chrome --action save_page --format pdf --path page.pdf
zagent_in_chrome --action end
```

All tools have `--help`, `--version`, `--json`, `--quiet`, `--config`, `--profile`, `--browser_path`, `--extension_id`, `--auto_launch`, and `--timeout_sec`. Flag names accept either underscores or hyphens (`--chat_id` and `--chat-id`). `web_search` also accepts `--max_results` as an alias for `--result_count`. Use `--flag=false` to disable a boolean. Repeat `--reference_image` or `--file` for multiple input files.

## Configuration

Keep `config.json` beside the executable. New configs contain only:

```json
{
  "extension_id": "PASTE_THE_FULL_CONNECTION_ID_FROM_THE_POPUP",
  "auto_launch": true,
  "profile": "Default",
  "path": "",
  "output_dir": "."
}
```

| Setting | Meaning |
| --- | --- |
| `extension_id` | Full Connection ID copied from this profile's extension popup, including its dot and secret suffix. It differs from Chrome's fixed extension ID. |
| `auto_launch` | Start the selected browser when the matching extension is offline. Default `true`. |
| `profile` | Browser profile folder, such as `Default` or `Profile 1`, from `chrome://version` under Profile Path. |
| `path` | Browser executable. Empty detects Chrome, Edge, Brave, or Chromium. Prefer an absolute path for a custom browser. |
| `output_dir` | Default file destination. `.` means the command's working directory. |

Each profile has a unique Connection ID. Use one config per profile and choose it with `--config FILE`. The ID selects the connected profile; `profile`, `path`, and `user_data_dir` control which browser launches when that profile is offline. After Reset ID, update the relevant configs.

Optional settings retain defaults, so most users need not add them:

| Advanced setting | Default | Meaning |
| --- | --- | --- |
| `browser_args` | `[]` | Extra browser arguments as separate JSON strings |
| `user_data_dir` | empty | Custom browser user-data directory |
| `state_dir` | empty | Media state root; otherwise the user's cache under `zpilot/cli` |
| `session` | `default` | Image/music series name |
| `image_chat_limit` | `20` | Successful images per chat before rotation, 1–200 |
| `music_chat_limit` | `10` | Successful tracks per chat before rotation, 1–100 |
| `result_count` | `10` | Search result count, 1–50 |
| `search_engine` | `google` | Default search engine |
| `image_timeout_sec` | `300` | Image timeout, 30–900 seconds |
| `music_timeout_sec` | `600` | Music timeout, 60–1500 seconds |
| `connect_timeout_sec` | `45` | Wait for the matching profile, 1–300 seconds |

CLI flags override config. `--extension_id` overrides the Connection ID for a call; `--browser_path` overrides the browser executable (`--path` remains the existing output-path flag). `--print_config` displays effective settings. `--init_config` creates the small default config without overwriting. A missing Connection ID produces a setup message; unknown or invalid fields fail explicitly.

Other relative configured output paths, `state_dir`, and `user_data_dir` resolve beside the config. Flag paths resolve from the current working directory. `chrome_profile` and `browser_path` remain supported aliases; do not specify both an old key and its new name. Remove obsolete `socket` fields from old configs; the native-host transport and `--socket` flag have been removed.

## Saved chats and files

`--session` selects a persistent local series. Run the same command again, even from another working directory, to continue its chat. State is separate for image/music tools and each configured Connection ID/profile/user-data combination. The saved `session_file` appears in JSON output. Changing config files alone does not create a new series when they select the same Connection ID and profile.

At the configured chat limit, the next call starts a new chat. Images carry the latest two existing files as references by default (`--carry_references 0` disables it); music restates the saved style brief. A `--style` value is remembered for the series. `--new_chat` forces rotation. `--temporary_chat` makes an unsaved ChatGPT chat and leaves the regular saved chat selected.

`--chat_id` accepts a bare ID or a full `https://chatgpt.com/c/<id>` / `https://gemini.google.com/app/<id>` URL. It pins that conversation for this call, even if the limit is reached, and selects it for the session's next call. It cannot be combined with `--new_chat` or `--temporary_chat`. Counts only include successful generations recorded by these CLIs; existing messages from manual use are not counted.

`generate_image` and `generate_music` use `--path` as a directory and `--name` as the filename. Images default to PNG and music to MP3. Image filename extensions choose the format unless `--format` explicitly conflicts. `--output` instead selects an exact unused filename, and cannot be combined with `--path`/`--name`. Without a name, the prompt supplies a sanitized filename. Automatic names get `-2`, `-3`, etc. when a file exists; exact output paths fail instead of replacing files. Each successful generation also saves metadata JSON, and music can include cover art and notes. Original Chrome downloads are preserved.

For `webpage_read`, `--path` is the content file and requests the full page, ignoring chunk flags. For `web_search`, it is a JSON results file. For browser screenshots/downloads/pages, it is an exact file, an existing directory, or a directory ending with a slash. Otherwise browser actions return their text/JSON result without creating a file.

Concurrent media commands for the same configured tool/profile are serialized with a filesystem lock to preserve sessions. Ctrl+C closes the CLI connection, which asks the extension to cancel the call. If a process is forcibly killed, inspect the reported `.lock/owner.txt` and remove the abandoned lock directory only after confirming that process is no longer running.

The extension routes requests by the per-profile Connection ID. Same-tool requests queue across clients; different tools can run in parallel. Reset ID cancels this profile's jobs and requires updating its tool configs. Queue time counts toward timeout_sec.

## Output, errors, and tests

Results go to stdout; progress and errors go to stderr. `--json` returns structured extension results with embedded image data replaced by saved paths, and structured tool errors preserving codes such as `RATE_LIMITED` and `LOGIN_REQUIRED`. Exit codes are `0` for success/help, `1` for execution/file/connection failures, and `2` for flag/config validation. `--quiet` suppresses progress. Commands do not retry generation automatically.

The tools operate in your signed-in browser. Use browser actions only for actions you intend to perform; page content is untrusted input when passing results into another automation system.

```powershell
go test -count=1 ./...
go vet ./...
```

Go tests cover direct HTTP routing, profile isolation, reset, cancellation, origin checks, CLI flags, media continuity, rotation, file output, structured errors, configuration, and state locks. CLI tests use a fake HTTP extension endpoint and temporary files; no native host or browser account is needed. Run extension queue/navigation tests with `node --test tests/*.test.mjs` inside `extension`. The browser smoke test in `extension/tests/browser-smoke.mjs` uses isolated profiles and local fixture pages. Live ChatGPT/Gemini generation still depends on their interfaces and your signed-in browser.
