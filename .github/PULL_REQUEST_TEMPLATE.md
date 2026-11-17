## Summary of Changes
<!-- Describe what tool was added or what changes were made -->

### Tool Information (If adding or updating a tool)
- **Tool ID:** 
- **Tool Name:** 
- **Category:** 
- **Manifest Path:** `tools/<tool_name>/<tool_name>.json`

### Contributor Checklist
Please check all that apply:
- [ ] Tool source code and manifest are placed under `tools/<tool_name>/`.
- [ ] Tool manifest `<tool_name>.json` is valid JSON and strictly follows [TOOL_SPECIFICATION.md](TOOL_SPECIFICATION.md).
- [ ] Tool has an entry added in `registry.json` conforming to `registry.schema.json`.
- [ ] No compiled binary files (`.exe`, `.dll`, `.so`, `.zip`) are committed to git tracking.
- [ ] Tool operates strictly within the workspace directory and respects timeouts.
- [ ] Tested locally with `zagent tools run <tool_name>` or CLI flags.
- [ ] `README.md` is provided in the tool directory explaining usage and options.
