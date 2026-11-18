# workspace_tree

> A blazing-fast, token-efficient workspace directory tree generator for ZAgent and autonomous engineering systems.

## Features

- **Noise Filtering:** Automatically omits developer noise folders (`.git`, `node_modules`, `build`, `dist`, `target`, `bin`, `.idea`, `__pycache__`, etc.).
- **Respects `.gitignore`:** Parses and excludes files matching project `.gitignore` patterns.
- **Context Preservation:** Enforces `max_depth` and safety caps (`max_entries`) to prevent context window explosion.
- **Filtering:** Filter by extensions (e.g. `[".go", ".ts"]`).
- **Multiple Formats:** Visual ASCII/Unicode tree, structured JSON, or flat relative paths.
- **ZAgent Protocol:** Conforms directly to the `--agent-json` I/O specification.

## Usage

### Standalone CLI
```bash
./workspace_tree -path . -depth 3 -sizes
```

### With ZAgent
```json
{
  "path": "./src",
  "max_depth": 3,
  "show_sizes": true,
  "extensions": [".go", ".mod"],
  "format": "tree"
}
```

```bash
zagent tools run workspace_tree --args '{"path": "./src", "max_depth": 3}'
```

## License
Apache-2.0
