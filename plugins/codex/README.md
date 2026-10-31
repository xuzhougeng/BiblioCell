# Codex / Claude plugin marketplace

Plugins published from this repository are declared at the repository root:

```
.claude-plugin/marketplace.json     Claude-compatible manifest (read by Claude Code and Codex)
.agents/plugins/marketplace.json    Codex-native manifest (same plugin list)
plugins/codex/mygo-maintenance/     the plugin itself
├── .codex-plugin/plugin.json       plugin metadata
└── skills/mygo-maintenance/
    ├── SKILL.md                    copy of /.agents/skills/mygo-maintenance/SKILL.md
    └── agents/openai.yaml
```

`SKILL.md` is kept in sync with `/.agents/skills/mygo-maintenance/`: edit the
source skill there and copy it into the plugin directory before publishing.

## Add the marketplace

Codex / ChatGPT desktop → Plugins → Add plugin marketplace:

| Field | Value |
| --- | --- |
| Source | `https://github.com/xuzhougeng/BiblioCell` |
| Git ref | `main` |
| Sparse path | leave empty (small repo), or `plugins/codex` for a partial checkout |

Or from the CLI:

```bash
codex plugin marketplace add xuzhougeng/BiblioCell --ref main
codex plugin add mygo-maintenance@biblio-cell
```

For Claude Code:

```bash
/plugin marketplace add xuzhougeng/BiblioCell
```
