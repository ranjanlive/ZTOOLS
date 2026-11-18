# git_changelog

> Automated Conventional Commits parser, SemVer bump calculator, and release notes generator for ZAgent and CI/CD pipelines.

## Features

- **Conventional Commits:** Automatically parses `feat:`, `fix:`, `perf:`, `docs:`, `refactor:`, `test:`, `chore:`.
- **Breaking Changes:** Detects `!` (e.g. `feat!:`) and `BREAKING CHANGE:` declarations.
- **SemVer Bump Prediction:** Calculates whether the next release should be a `major`, `minor`, or `patch` bump.
- **Multiple Formats:** Outputs GitHub-ready Markdown release sections or structured JSON.
- **ZAgent Protocol:** Conforms directly to the `--agent-json` I/O specification.

## Usage

### Standalone CLI
```bash
./git_changelog -repo . -version v1.0.0
```

### With ZAgent
```json
{
  "repo_path": ".",
  "current_version": "v1.0.0",
  "format": "markdown",
  "include_hashes": true
}
```

```bash
zagent tools run git_changelog --args '{"current_version": "v1.0.0"}'
```

## License
Apache-2.0
