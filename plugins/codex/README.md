# BiblioCell Codex plugin marketplace

Codex / ChatGPT desktop plugins published from this repository. Everything lives
under `plugins/codex`, which is also the sparse checkout path to use when adding
the marketplace from Git.

## Layout

```
plugins/codex/
├── .agents/plugins/marketplace.json   # Codex-native manifest
├── .claude-plugin/marketplace.json    # Claude-compatible manifest (legacy format)
└── mygo-maintenance/
    ├── .codex-plugin/plugin.json      # plugin metadata
    └── skills/mygo-maintenance/
        ├── SKILL.md                   # copy of /.agents/skills/mygo-maintenance/SKILL.md
        └── agents/openai.yaml
```

`SKILL.md` is kept in sync with `/.agents/skills/mygo-maintenance/`: edit the
source skill there and copy it into the plugin directory before publishing.

## Add the marketplace

Codex / ChatGPT desktop → Settings → Plugins → Add plugin marketplace:

| Field | Value |
| --- | --- |
| Source | `https://github.com/xuzhougeng/BiblioCell` |
| Git ref | `main` |
| Sparse path | `plugins/codex` |

Or from the CLI:

```bash
codex plugin marketplace add xuzhougeng/BiblioCell --ref main --sparse plugins/codex
codex plugin add mygo-maintenance@biblio-cell
```

For Claude Code:

```bash
/plugin marketplace add xuzhougeng/BiblioCell
```
