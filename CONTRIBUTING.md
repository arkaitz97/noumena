# Contributing

This repository is a personal project. Issues and small pull requests are
welcome. Large redesigns are unlikely to be merged; open an issue first.

## Before you write code

1. Read [docs/design.md](docs/design.md) and the relevant ADR in
   [docs/adr/](docs/adr/).
2. Search the code for the behaviour you intend to change. The read paths and
   ingestion paths have invariants documented at the top of the file.
3. For a bug, reproduce it first. State the command, the expected result, and
   the actual result in the pull request.

## Tests are required

A change to behaviour needs a test that fails without the change. No
exceptions for "obvious" fixes.

Run the same gates as CI:

```sh
make fmt-check          # gofmt gate
go vet ./...
go test ./...           # in-memory SQLite, no services
make test-integration   # race detector on the db packages
bash scripts/test-frontend.sh
```

Name tests explicitly. Prefer `TestStripeWebhookDuplicateShortCircuits` over
`TestWebhook2`. A test name is documentation for the next failure.

If a change cannot be tested, say why in the pull request. If it needs a
manual check, add the step to the relevant runbook under `docs/ops/runbooks/`.

## Small pull requests

One logical change per pull request. A migration, a refactor, and a feature do
not belong together. If the diff needs a tour to review it, split it.

Keep the diff close to the change. Do not reformat untouched files, rename
unrelated symbols, or upgrade dependencies in a feature pull request.

## Commit messages

Use Conventional Commits:

```
<type>(<scope>): <subject>
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `perf`, `build`,
`ci`. Scope is the package or area: `api`, `db`, `ingestion`, `llm`,
`billing`, `lifecycle`, `frontend`, `docs`.

Subject in the imperative mood, 72 characters or fewer, no trailing period.
The body explains why, not what. The diff already shows what.

Examples:

```
feat(workspaces): authenticated invite acceptance flow
fix(billing): reject duplicate webhook event before dispatch
docs(adr): record the SQLite single-writer decision
```

## Review expectations

- A reviewer reads the diff and runs the tests. Do not ask for review before
  CI is green.
- Address every comment or reply with a reason. "Fixed" without a commit is
  not a resolution.
- Do not merge your own pull request without a review, except for a
  documentation typo.
- Keep the branch rebased on `main`. No merge commits in a feature branch.

## Code style

- Guard clauses over nesting. Return early on error and on the negative case.
  A function with three levels of `if` is a function that needs extraction.
- One access-control predicate, one feed builder. Do not add a new comment
  read path outside `BuildFeedQuery`.
- Wrap errors with context: `fmt.Errorf("claim job: %w", err)`.
- Never log a token, a password, a query string, or a full request body.
- Keep SQL in `internal/db`. Handlers depend on interfaces.
- Comments explain why. The code explains what.

## Security

Do not open a public issue for a vulnerability. Follow
[SECURITY.md](SECURITY.md).

## License

By contributing, you agree that your contribution is licensed under the MIT
License in [LICENSE](LICENSE).
