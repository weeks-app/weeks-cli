# Contributing to weeks-cli

## Setup

```bash
mise install          # Go, pinned in mise.toml
make build
make check
```

`make test-e2e` needs [bats](https://github.com/bats-core/bats-core)
(`brew install bats-core`) and `jq`. `make check-surface` needs `jq`.

## Before you open a PR

```bash
make check-all
```

That is what CI runs: formatting, vet, lint, the race detector, the e2e suite,
and the CLI surface snapshot.

## What reviewers look for

- **Breadcrumbs.** A command that returns data without suggesting what comes
  next is a dead end for the agent that called it.
- **A `hint` on every failure.** `weeks doctor` asserts this for its own checks;
  hold command errors to the same bar.
- **The skill.** If a change alters what an agent needs to know,
  `skills/weeks/SKILL.md` changes in the same PR.
- **The surface.** Adding a flag or a command is free. Removing one — including
  an alias — breaks callers that already exist, so say why in the PR body. CI
  will flag it either way.

## Testing

- Unit tests live beside the code and use the real thing where they can: the
  device flow is tested against an `httptest` server that speaks RFC 8628,
  not a mock of our own client.
- `e2e/contract.bats` drives the built binary the way a caller meets it. That
  is the only place the envelope, the exit codes, and the catalog are tested
  *together*, which is how they are actually used.
- Tests must not touch a developer's real credentials. Set `WEEKS_CONFIG_DIR`
  and `WEEKS_NO_KEYRING`, as the bats suite does.

## Commits

Write the message for someone reading `git log` a year from now: what changed,
and why it was worth changing. PR into `main`; merge with a merge commit.
