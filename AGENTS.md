# weeks-cli — for agents working *on* this repo

This is the Go CLI and agent-skill face of weeks. If you are trying to *use*
the CLI rather than change it, read `skills/weeks/SKILL.md` — or run
`weeks skill`, which prints the same document out of the binary.

## What this repo is

A thin, honest surface over the weeks API, built on the MIT 37signals toolkit
`github.com/basecamp/cli`. The toolkit owns the envelope, the credential store,
PKCE, profiles, and the surface-diff tooling. This repo owns the weeks
vocabulary, the device flow, and the confirmation gate.

The design rule from the epic (`weeks-app-s0mo`): **one action model = one CLI
verb = one API endpoint = one agent tool = one UI form.** When a command here
does something the app can also do, it must call the same action, not a
parallel implementation.

## Toolchain

Go is pinned in `mise.toml`. A shell that has half-activated mise builds
against one toolchain and links against another; `make check-toolchain` catches
that, and every build target depends on it.

```bash
make check      # gofmt, vet, tests, tidiness — the inner loop
make check-all  # what CI runs: lint, race detector, e2e, surface snapshot
make build      # ./bin/weeks
make test-e2e   # bats, against the built binary
```

## The contract is the product

Three things are load-bearing and must not drift:

1. **The envelope.** `{ok, data, summary, breadcrumbs}` on stdout, with the
   error shape never setting `ok` to true. Covered by
   `internal/output/output_test.go` and `e2e/contract.bats`.
2. **The exit codes.** They always agree with the `code` field.
   `confirmation_required` is weeks-specific and sits at 9, one past the
   toolkit's highest, so it can never be mistaken for a server error.
3. **The surface.** Removing a flag, a subcommand, or an alias breaks a plan an
   agent already made. `scripts/check-cli-surface.sh` snapshots it and CI
   compares against `main` on every PR. Adding is free; removing needs a reason
   in the PR body.

`--help --agent` emits a **bare** JSON object, not the envelope. That is the
toolkit's convention and it is load-bearing: the surface script and the
toolkit's rubric-check and surface-compat actions read `.flags` and
`.subcommands` off the top level.

Anything an agent must know to drive the CLI belongs in
`skills/weeks/SKILL.md`. A command whose behaviour the skill does not describe
is a command an agent will rediscover, badly, every session.

## Adding a command

1. A new file in `internal/commands`, returning a `*cobra.Command`.
2. Read the invocation from `appctx.From(cmd)` — never re-derive the profile,
   base URL, or credential store from flags and environment.
3. Answer through `app.Out.OK(...)` with a `summary` and **breadcrumbs naming
   the commands that usually come next**. Breadcrumbs are how an agent plans;
   a command without them is a dead end.
4. Fail with `*output.Error`, which carries the code, so the exit status is
   right without the caller doing anything.
5. Register it in `NewRootCmd`. The catalog and `--help --agent` pick it up
   from the tree — there is no list to update, and deliberately so.
6. Teach it in `skills/weeks/SKILL.md` if an agent would need to know.

## Auth

`internal/auth` speaks to weeks-app's Doorkeeper provider:

- **Device flow (RFC 8628)** is the default, because it is the one that works
  where agents run: no browser on the host, no redirect back to it.
- **Authorization code + PKCE** is `--browser`, for a desktop.

Both need weeks-app to support them. Device flow needs
`doorkeeper-device_authorization_grant`; PKCE needs the `code_challenge`
columns on `oauth_access_grants`. Neither was present before this work — see
the weeks-app PR that adds them.

`Client.Sleep` exists so the poll loop's timing (the RFC 8628 back-off, the
deadline) can be tested without spending real seconds. Leave it nil in
production.

## Working with weeks-app

In a worktree collection, weeks-app serves on the collection's
`WEEKS_APP_PORT`, from `../.env.collection`:

```bash
set -a; . ../.env.collection; set +a
export WEEKS_BASE_URL="http://localhost:${WEEKS_APP_PORT}"
```

This repo has no `port_offset` of its own — it serves nothing, it consumes
weeks-app's port. Same footing as `weeks-ios`.

## Branch and PR policy

The collection's policy applies: worktrees rest detached at `origin/main`,
branch at the first commit, PR into `main`, merge with a merge commit — never
squash, never rebase.
