# weeks-cli

The command-line and agent interface to [weeks](https://github.com/weeks-app/weeks-app),
a staff-scheduling app. Built on the MIT
[37signals Go CLI toolkit](https://github.com/basecamp/cli).

It is designed to be driven by an AI agent as readily as by a person: every
command answers with the same JSON envelope, every answer suggests what to do
next, and the agent skill that teaches the CLI ships inside the binary.

> **Status: bootstrap.** Auth, discovery, diagnostics, and the output contract
> are in place. The scheduling commands themselves — plans, slots, people,
> jobs, rollups, overlaps — are the next bean (`weeks-app-d3ya`).

## Install

With Homebrew:

```bash
brew tap weeks-app/tap
brew trust weeks-app/tap
brew install weeks
```

With the installer:

```bash
curl -fsSL https://about.weeks.app/install.sh | sh
```

The installer verifies the release checksum before placing `weeks` in
`~/.local/bin` by default. Set `WEEKS_INSTALL_DIR` or `WEEKS_INSTALL_VERSION`
to choose another directory or release.

From source:

```bash
go install github.com/weeks-app/weeks-cli/cmd/weeks@latest
```

## Getting started

```bash
weeks setup
weeks doctor --json
```

`weeks auth login` runs the OAuth device flow: it prints a short code and a
URL, you approve it in any browser on any machine, and the CLI picks up the
token. Nothing has to redirect back to the host running the command — which is
what makes it work over SSH, in a container, or in an agent's pane. Where there
is a desktop, `weeks auth login --browser` uses the authorization code grant
with PKCE instead.

## The contract

On a terminal, `weeks` prints a readable summary with the follow-up commands
under it. Everywhere else — a pipe, a script, an agent — it prints the JSON
envelope. Both are projections of the same answer, so they cannot disagree
about what happened.

Every command emits this on stdout when stdout is not a terminal, or whenever
`--json` is passed:

```json
{
  "ok": true,
  "data": {},
  "summary": "One sentence a human can read.",
  "breadcrumbs": [
    {"action": "verify", "cmd": "weeks auth status", "description": "Confirm the credential works"}
  ]
}
```

Failures never set `ok` to true:

```json
{"ok": false, "error": "what went wrong", "code": "auth_required", "hint": "what to do about it"}
```

| Flag | Effect |
|---|---|
| `--json` | Emit the envelope even on a terminal |
| `--quiet` | Emit `data` alone, for piping into `jq` |
| `--ids-only` | One id per line |
| `--count` | The number of results |
| `--agent` | The shape an agent reads: JSON output *and* structured help |
| `--profile` | Act as a named profile |
| `--confirm` | Proceed past a `confirmation_required` gate |

### Exit codes

The exit code and the `code` field always agree.

| Code | Exit | Meaning |
|---|---|---|
| `usage` | 1 | The command or its flags were wrong |
| `not_found` | 2 | No such record |
| `auth_required` | 3 | Not signed in, or the token expired |
| `forbidden` | 4 | Authenticated but not permitted |
| `rate_limit` | 5 | Back off and retry |
| `network` | 6 | The installation was unreachable |
| `api_error` | 7 | The server refused the request |
| `ambiguous` | 8 | A name matched several records |
| `confirmation_required` | 9 | A gate — see below |

`confirmation_required` is weeks' own addition to the toolkit's set. It is not
a failure: it means the operation is legal but crosses a gate the product also
enforces in its interface — a staffing overlap, a negative planning context.
The contract is exact: re-run the identical command with `--confirm`. It sits
at exit 9, one past the toolkit's highest, so it can never be confused with a
server error.

## Discovery

```bash
weeks commands --json          # the whole catalog, read from the binary itself
weeks commands --json --flat   # just the command paths
weeks <command> --help --agent # structured help for one command
weeks skill                    # the agent skill embedded in this binary
weeks setup claude             # install that skill for Claude Code
```

The catalog is derived from the live command tree, so it cannot drift from what
the binary can actually do. That is the point of publishing it on demand rather
than making an agent carry a tool list in its context forever.

## Storage Scope

By default, `weeks` stores profiles and credentials in `./.weeks/` for the
folder where the command runs. That keeps an agent working in one project from
silently reusing a login from another project. Local credentials are written to
`./.weeks/credentials.json` with mode 0600.

Use `--global` when you deliberately want the user-wide config under
`$XDG_CONFIG_HOME/weeks` or `~/.config/weeks`; global credentials prefer the
system keyring and fall back to a 0600 file.

```bash
weeks setup                 # writes ./.weeks/config.json and credentials.json
weeks --global setup        # writes the user-wide profile and credential
weeks --global teams list   # reads the global login
```

Folder-local storage can also remember a default team and space for read
commands. In a terminal, `weeks defaults set` lets you choose from the teams
and spaces your login can see; in scripts, pass the ids explicitly.

```bash
weeks defaults set
weeks defaults set --team team_abc --space space_abc
weeks defaults show
```

## Profiles are the team boundary

Credentials are stored per profile inside the selected storage scope. One
profile can never read another's token, so `weeks --profile acme` and
`weeks --profile beta` are separated; using folder-local storage also separates
agents by working directory.

```bash
weeks profile set beta --base-url https://weeks.app
weeks auth login --profile beta
weeks --profile beta doctor --json
```

## Environment

| Variable | Effect |
|---|---|
| `WEEKS_BASE_URL` | Target installation, overriding the profile |
| `WEEKS_PROFILE` | Profile to use, without `--profile` |
| `WEEKS_CLIENT_ID` | OAuth client id override; self-hosted or local weeks installations need a client id from this env var or the active profile |
| `WEEKS_CONFIG_DIR` | Where config and file-backed credentials live |
| `WEEKS_NO_KEYRING` | Force file-backed credential storage |

Commands that need no credential — `version`, `commands`, `skill`, `--help` —
never open the credential store, so a locked or absent keyring cannot stall
discovery.

Inside a weeks worktree collection, `WEEKS_BASE_URL` should point at the
collection's own `WEEKS_APP_PORT` — see `AGENTS.md`.

## Development

```bash
make check      # gofmt, vet, tests, module tidiness
make check-all  # everything CI runs, including the race detector and lint
make build
make test-e2e   # the agent contract, through the built binary (needs bats)
```

## License

MIT. The 37signals toolkit this is built on is also MIT
([basecamp/cli](https://github.com/basecamp/cli)).
