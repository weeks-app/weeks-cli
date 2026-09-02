---
name: weeks
description: Plan and staff work in weeks from the command line. Use for weeks scheduling data — spaces, plans, slots, assigned people and jobs, planning contexts, staffing overlaps, and rollups — via the `weeks` CLI. Triggers on "weeks", "staffing", "who is scheduled", "slot", "assigned job", "planning context", "overlap", "rollup".
---

# weeks

`weeks` is the command-line interface to weeks, a staff-scheduling app. It is
built to be driven by an agent: every command answers with the same JSON
envelope, and every answer suggests what to do next.

**This binary is early.** Auth, discovery, diagnostics, and basic read commands
for spaces and plans work. Most scheduling commands do not exist yet. `weeks
commands --json` is always the truth about what this binary can do — trust it
over any example in this document.

## Before anything else

```bash
weeks doctor --json      # config, credentials, connectivity — run this when something surprises you
weeks commands --json    # the full command catalog, read from the binary itself
```

`weeks commands --json` is the discovery surface. Call it once at the start of
a task rather than carrying a command list around: it is derived from the
binary's own command tree, so it is always exactly what this binary can do.
For one command, `weeks <command> --help --agent` returns the same structured
entry.

For a new machine, run:

```bash
weeks setup --profile default
```

In a terminal, it creates or selects the default profile in the current
folder's `./.weeks/` directory, installs this embedded skill for Claude Code,
and starts the device login flow. Local credentials are file-backed in
`./.weeks/credentials.json` so another folder's agent cannot silently reuse
them. Add root-position `--global` only when you deliberately want the
user-wide config and keyring-preferred credentials, for example
`weeks --global setup`. Add `--base-url` or `--client-id` when
configuring a non-hosted installation. Use `--skip-login` when you only want
setup to write profile and skill files. In JSON or other non-interactive runs,
setup never starts login unless you pass `--login`.

## Two shapes, and which one you get

`weeks` answers a person and an agent differently. On a terminal it prints a
readable summary; anywhere else — a pipe, a subprocess, a capture — it prints
the JSON envelope. **You will almost always get the envelope**, because an
agent is not a terminal. Pass `--json` to be certain of it.

The two are projections of the same answer, so they never disagree about what
happened. Everything below describes the envelope, which is the one to program
against.

## The envelope

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

- **`ok`** — whether the command succeeded. Check this first.
- **`data`** — the payload. An object for one thing, an array for a listing.
- **`summary`** — one sentence. Use it verbatim when reporting back.
- **`breadcrumbs`** — the commands that usually come next. They are the cheapest
  way to plan: follow one rather than guessing at a command that may not exist.

Failures use a different shape, and never set `ok` to true:

```json
{"ok": false, "error": "what went wrong", "code": "auth_required", "hint": "what to do about it"}
```

Pass `--quiet` to get `data` alone, with no envelope, for piping into `jq`.
`--ids-only` prints one id per line; `--count` prints the number of results.

## Error codes and exit codes

Branch on the `code`, or on the exit status — they always agree.

| Code | Exit | What it means |
|---|---|---|
| `usage` | 1 | The command or its flags were wrong. Re-read `--help --agent`. |
| `not_found` | 2 | No such record. Check the id, or list first. |
| `auth_required` | 3 | Not signed in, or the token expired. Run `weeks auth login`. |
| `forbidden` | 4 | Authenticated, but not permitted. Do not retry. |
| `rate_limit` | 5 | Back off and retry; `hint` says how long. |
| `network` | 6 | The installation was unreachable. Retryable. |
| `api_error` | 7 | The server refused the request. |
| `ambiguous` | 8 | A name matched several records. Use an id. |
| `confirmation_required` | 9 | **A gate. See below.** |

## The confirmation gate

`confirmation_required` is not a failure. It means the operation is legal and
understood, but it crosses a gate that weeks also enforces in its own
interface — a staffing overlap, a negative planning context, someone being put
somewhere the plan says they should not be.

The contract is exact: **re-run the identical command with `--confirm`.**

The command below is an illustration, not a command this binary has yet — the
gate is what to learn, and it will look like this wherever it appears:

```bash
weeks assign --person 42 --to-slot 17
# {"ok": false, "code": "confirmation_required",
#  "error": "Dana is already assigned to an overlapping slot on Tuesday",
#  "hint": "Re-run the same command with --confirm to proceed."}

weeks assign --person 42 --to-slot 17 --confirm
```

Do not paper over a gate. The `error` names a real conflict a planner would
want to know about — surface it before you clear it, and say what you cleared.

## Signing in

```bash
weeks auth login          # device flow: shows a code and a URL, works over SSH
weeks auth login --browser # authorization code + PKCE, when there is a desktop
weeks auth status
```

The device flow is the default because it is the one that works where agents
run: it prints a short code, you approve it in any browser on any machine, and
the CLI picks the token up. Nothing needs to redirect back to this host, and
nothing opens a browser on its own — pressing Enter offers to open one here,
and ignoring that prompt is a normal way to finish, because the approval
usually happens somewhere else.

The hosted `https://weeks.app` installation has a built-in public OAuth client
id. Local and self-hosted installations issue their own client id; set
`WEEKS_CLIENT_ID` or store it on the profile with `weeks profile set
--client-id`.

## Profiles are the team boundary

```bash
weeks profile set acme --base-url https://weeks.app
weeks auth login --profile acme
weeks --profile acme <command>
```

Credentials are stored per profile, so one profile can never read another's
token. When work spans two teams, use two profiles — never one credential with
wider access.

## Basic reads

```bash
weeks teams list
weeks teams view <team-id>

weeks spaces list
weeks spaces list --team <team-id>
weeks spaces view <space-id>
weeks spaces view <space-id> --include overview

weeks plans list --space <space-id>
weeks plans view <plan-id>
weeks plans view <plan-id> --include snapshot
```

`view` means a GET of one API resource. JSON output preserves the resource
shape the API returned. Human output is a compact projection of the same data:
name, id, important references, counts, and collection sizes.

Use typed IDs exactly as the API returns them, such as `team_...`, `space_...`,
and `plan_...`. `weeks spaces list` defaults the team only when the profile can
access exactly one team; otherwise run `weeks teams list` and pass
`--team <team-id>`. Listing plans needs a space id because plans live inside a
space.

Include scopes are passed straight to the API. Useful starting points:

- `spaces view --include counts` for people and plan counts.
- `spaces view --include overview` for counts, people, plans, and plan counts.
- `plans view --include counts` for people, jobs, slots, and inbox counts.
- `plans view --include snapshot` for people, jobs, inboxes, slots, assignments,
  routes, and related planning hints.

## Vocabulary

These are the words the product uses; say them back to the user.

- **Space** — the top-level container a team plans inside.
- **Plan** — a schedule within a space: a production, an event, a period.
- **Slot** — a span of time in a plan that needs people. Slots are what get
  staffed, and what overlap with each other.
- **Assigned person** — someone placed in a slot.
- **Assigned job** — a role within a slot that people are assigned to.
- **Planning context** — a condition on a person's availability. It can be
  negative: "not on weekends", "not with this crew". Contexts are what the
  confirmation gate is usually protecting.
- **Overlap** — two slots wanting the same person at the same time.
- **Rollup** — a plan's staffing state summarized: what is filled, what is
  short, what conflicts.

## Working habits

- Run `weeks commands --json` first; do not guess at command names. If a
  command you expected is missing, it has not shipped — say so rather than
  improvising around it.
- Follow the breadcrumbs. They encode what the CLI expects you to need next.
- Report the `summary` rather than re-describing the JSON.
- When a command fails, read `hint` before trying anything else. It names the
  fix nearly every time.
- When a gate fires, tell the user what the conflict is, then decide with them
  whether to `--confirm`.
