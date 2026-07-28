# pkg/fileops

Shared file-operation core for Echoryn. Consumed by:

- `internal/hivemind/service/plugin/builtin/local-fileops` — Hivemind local tools
- `internal/golem/skills/fileops` — Golem remote skills

## API Surface

| Function | Purpose |
|----------|---------|
| `ReadFile(sb, path, offset, limit)` | Numbered lines, pagination, binary detection, similar-file hints |
| `ReadFileRaw(sb, path)` | Full content, no pagination |
| `WriteFile(sb, path, content)` | Auto-mkdir, overwrites existing |
| `PatchReplace(sb, path, old, new, all)` | Find-and-replace with fuzzy fallback, unified diff |
| `Search(sb, opts)` | ripgrep-like content (regex) / file (glob) search |

All results carry `Error string` for soft failures (sandbox denial, file not
found). Callers can JSON-serialize results directly for LLM tool output.

## Sandbox

```go
sb := &fileops.Sandbox{
    ReadAllowedRoots:  []string{"/workspace"},
    WriteEnabled:      true,
    WriteAllowedRoots: []string{"/workspace"},
}
```

- Nil Sandbox → reads allowed (builtin deny only), writes denied.
- Zero value of `ReadAllowedRoots` / `WriteAllowedRoots` → no root restriction.
- `DenyPathsExact` / `DenyPathsPrefix` add to the builtin deny list.
- Symlinks are resolved during path check (including the macOS
  `/etc → /private/etc` case); deny matches the resolved realpath.

## Fuzzy matching (PatchReplace)

When `replaceAll=false` and the exact literal isn't unique, `PatchReplace`
falls back to `FindBestMatch` which tries 4 strategies in order:

1. Exact substring.
2. Per-line whitespace-trimmed.
3. Line-ending normalized (`\r\n` / `\r` → `\n`).
4. Indent-normalized (strip leading whitespace).

## Adding new operations

1. Add result struct in `types.go`.
2. Implement in its own file + test file (TDD).
3. Always call `sb.CheckRead` / `sb.CheckWrite` before I/O.
4. Encode errors in the result struct's `Error` field; do not return Go
   errors from user-facing operations.
