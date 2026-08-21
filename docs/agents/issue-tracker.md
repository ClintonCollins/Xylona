# Issue tracker: GitHub

Issues and specs for this repo live as GitHub issues. Use the `gh` CLI from inside the clone so it resolves the repository automatically.

## Conventions

- **Create an issue**: `gh issue create --title "..." --body "..."`. For multi-line bodies, use `--body-file <path>` or `--body-file -` with stdin.
- **Read an issue**: `gh issue view <number> --json number,title,body,labels,comments`. Add `--jq` only when filtering is useful.
- **List issues**: `gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'` with appropriate `--label` and `--state` filters.
- **Comment on an issue**: `gh issue comment <number> --body "..."`
- **Apply / remove labels**: `gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **Close**: `gh issue close <number> --comment "..."`

Treat “publish to the issue tracker” as creating a GitHub issue. Treat “fetch the relevant ticket” as `gh issue view <number> --json number,title,body,labels,comments`.
