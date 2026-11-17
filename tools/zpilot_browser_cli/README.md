# ZPilot Browser CLI 2.0

Five independent tool binaries connect to your browser extension using a Connection ID. **No zbrowser-host install, native-messaging registration, or separate service is needed.**

## Quick setup

1. Open `chrome://extensions` in your intended browser profile. Enable **Developer mode**, select **Load unpacked**, and choose this package's `extension` folder. Reload it if already installed.
2. Open the ZPilot Browser popup and click **Copy** beside **Connection ID**.
3. Open `config.json` beside the tool binary and paste that ID into `extension_id`:

   ```json
   {
     "extension_id": "PASTE_THE_FULL_CONNECTION_ID_FROM_THE_POPUP",
     "auto_launch": true,
     "profile": "Default",
     "path": "",
     "output_dir": "."
   }
   ```

4. Run your tool:

   ```powershell
   .\web_search\binaries\windows-amd64\web_search.exe --query "Go tutorials"
   .\webpage_read\binaries\windows-amd64\webpage_read.exe --url "https://example.com"
   ```

`path` is the **browser executable**; leave empty for automatic detection. `profile` is its folder name, such as `Default` or `Profile 1`, shown in `chrome://version` under Profile Path. `output_dir` controls saved files; `.` means your current working directory. Sign in to ChatGPT/Gemini in the selected profile before using generation tools.

Copy just one tool's matching binary and config into any folder to use it independently. You do not need the other tool binaries, Go, Node.js, or Python. Keep the extension folder loaded in your browser. On Linux/macOS, make the tool executable with `chmod +x`.

## Profiles and reset

Each browser profile gets a separate Connection ID even when Chrome shows the same extension ID in both profiles. Use the desired profile's Connection ID in each tool's config. Chrome's fixed extension ID alone cannot identify a profile or authorize an executable.

The popup's **Reset ID** button creates a new Connection ID for that profile and cancels its pending/running requests. Copy the new ID into the relevant tool configs. Other profiles keep their IDs. Reset also separates new media-session storage from the old identity; old session files are preserved.

The Connection ID is a local access credential. Keep it in your config rather than posting it publicly. It is stored locally, not synced between profiles or sent to a remote service.

## Queues and connections

- Requests for the **same tool** run in arrival order across all clients of a profile.
- **Different tools** can run in parallel. Image generation does not block searches or page reading.
- The popup shows running and queued requests. Queue waiting counts toward the request timeout.
- Ctrl+C cancels the command. A cancelled queued request does not execute later.

Every tool embeds the same localhost connection code. The first command automatically starts a hidden copy of **itself** in connection mode on `127.0.0.1:19280`; other tools can reuse it. It routes by Connection ID and exits after 60 seconds without tool requests. Nothing is installed as a background service. Moving a binary needs no browser registration.

The extension connects locally. When its idle service worker sleeps, opening the popup reconnects immediately; an alarm also wakes it within 30 seconds. `auto_launch: true` opens the selected browser profile and an extension connection page when needed. `auto_launch: false` leaves browser startup to you. If port 19280 is occupied by another application, free that port and retry.

## Optional installer

Setup only copies the tools/extension and optionally adds launchers to PATH. It does not install or register a host.

```powershell
.\setup.ps1
# After loading the installed extension and copying its Connection ID:
.\setup.ps1 -ExtensionID "PASTE_CONNECTION_ID" -Profile "Profile 1"
```

The second command applies the ID/profile to all five installed configs. Use `-BrowserPath` for a custom browser executable or `-InstallDir` for a custom destination. Existing configs are preserved unless you explicitly supply connection/profile/path overrides. `-NoPath` skips PATH changes. The default destination is `%LOCALAPPDATA%\ZPilotBrowserCLI`.

```sh
sh setup.sh
sh setup.sh --prefix "$HOME/Tools/ZPilot" --no-path
```

Linux/macOS default to `~/.local/share/zpilot-browser-cli`. Open a new terminal after installing. Load the **installed** extension folder printed by setup.

On upgrade, reload the extension and replace the tool binaries together. Old host registrations can remain unused; this package contains no native host. Old `chrome_profile` and `browser_path` config names still load; prefer `profile` and `path`. Remove any obsolete `socket` field from an older config.

## Project layout

```text
extension/                 Browser extension and automated tests
web_search/                Portable binaries, configs, and independent Go source
webpage_read/              Same structure
generate_image/            Same structure
generate_music/            Same structure
zagent_in_chrome/          Same structure
scripts/                   Rebuild tools and refresh release checksums
agent_tools/               Canonical ZAgent JSON manifests and installation guide
setup.ps1 / setup.cmd      Optional Windows installation
setup.sh                   Optional Linux/macOS installation
CLI-USAGE.md               Commands and configuration reference
SHA256SUMS                 Distribution checksums
```

Source files under each tool's `source_code` and configs under each platform folder are intentional: a source module or binary folder can be copied and used independently.

## Build and test

Each `tool/source_code` directory is a self-contained Go module, requiring Go 1.24 or newer to build:

```sh
cd web_search/source_code
go test ./...
go vet ./...
go build -trimpath -o web_search .
```

On Windows use `-o web_search.exe`. Build all five tools for Windows/Linux/macOS, amd64/arm64, from PowerShell:

```powershell
.\scripts\build.ps1
# Or only this platform:
.\scripts\build.ps1 -CurrentPlatformOnly
```

The release script copies the canonical shared code from `web_search/source_code/internal/browsercli` to the other self-contained modules before building. Maintain shared changes there. It copies each tool's simple config into its platform folders.

Extension tests: `cd extension` then `node --test tests/*.test.mjs`.

Browser smoke test: `node extension/tests/browser-smoke.mjs`, using Playwright and an extension-capable Chromium. Set `PLAYWRIGHT_MODULE` to Playwright's `index.mjs` and `CHROMIUM_PATH` to Chromium if needed. It uses isolated temporary profiles and local pages, checking independent startup, profile routing, queues, parallelism, and ID reset. It does not submit generation prompts or use personal profiles.

Cross-compilation validates builds; runtime behavior on Linux, macOS, and ARM needs testing on those systems. `SHA256SUMS` covers distribution files; it detects changes rather than providing a signing certificate.

See [CLI-USAGE.md](CLI-USAGE.md) for commands, advanced settings, saved sessions, and output behavior.

## AI agent tools

Every platform binary has a matching `<tool>.json` manifest and `config.json` beside it. Copy those three files into `<ZAGENT_HOME>/tools/<tool>/`. JSON input/output handling is compiled into the binary; Node.js, Python, and wrapper scripts are unnecessary at runtime. The relative `{dir}/<tool>` command launches the adjacent binary and resolves `.exe` on Windows. See the [agent tool installation guide](agent_tools/README.md).

For development, regenerate manifests with `node scripts/generate-agent-tools.cjs` and validate them with `node --test scripts/agent-tools.test.cjs`. These development commands require Node; using the generated tools does not. Package this native bundle with `./scripts/package.ps1 -Variant native-agent-tools`.
