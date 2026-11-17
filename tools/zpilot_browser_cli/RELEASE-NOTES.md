# ZPilot Browser CLI 2.0.0

This package includes five independent tools, each with self-contained Go source and binaries for Windows, Linux, and macOS on amd64 and arm64. The Chromium extension, setup scripts, build scripts, configuration templates, and documentation are included.

## Changes

- Connect using the profile-specific Connection ID copied from the extension popup.
- Each tool embeds its connection bridge; no native host installation or registration is needed.
- Same-tool requests queue; different tools can run in parallel.
- The popup provides Copy and Reset ID controls.
- New configs use `extension_id`, `auto_launch`, `profile`, `path`, and `output_dir`.
- Obsolete native-host files and socket transport have been removed.
- Every platform folder includes a ZAgent JSON manifest beside its binary and config. The binary's native `--agent-json` mode handles all CLI arguments, JSON results, output-folder routing, media attachments, and progress notes without Node.js, Python, or wrapper scripts. Relative manifest paths allow the three files to move together.

## Validation and known limitations

All five Go test suites and static checks, six extension tests, and cross-compilation for all six targets passed. Browser smoke tests on Windows exercised independent connections, profile isolation, queues, parallelism, and ID reset.

**Known issue:** repeated browser page reads intermittently report `NAVIGATION_ABORTED`. The final browser smoke run encountered this issue; navigation reliability needs further work before treating this build as production-stable.

Linux, macOS, and ARM binaries are cross-compiled and have not been runtime-tested on those platforms. Live ChatGPT image generation and Gemini music generation were not retested during this release preparation.

Start with README.md for setup and CLI-USAGE.md for commands. Reload the extension when upgrading. Copy the popup's full Connection ID into each tool's config; Chrome's fixed extension ID is not the Connection ID.
