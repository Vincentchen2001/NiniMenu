# Agent Instructions

Before making changes in this repository, read:

```text
CODEX_CONTEXT.md
```

That file records the fork setup, custom commits, local run commands, verification
history, and the lunch-count change that allows lunch to be skipped.

Key repository assumptions:

- `origin` should point to `https://github.com/Vincentchen2001/NiniMenu.git`.
- `upstream` should point to `https://github.com/TryHarder-L/NiniMenu.git`.
- Keep local runtime data, databases, logs, and generated dev artifacts out of
  commits.
- Check `git status -sb` before committing.
- When syncing upstream changes, use `git fetch upstream` and merge
  `upstream/main`.

