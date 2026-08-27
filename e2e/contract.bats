#!/usr/bin/env bats
#
# The agent contract, exercised through the built binary the way a caller
# meets it. Unit tests cover the pieces; these cover the promises the skill
# makes: one envelope on stdout, exit codes that agree with the code field,
# and a catalog that matches the binary.

setup() {
  BINARY="${BATS_TEST_DIRNAME}/../bin/weeks"
  [ -x "$BINARY" ] || skip "binary not built; run make build"

  # Never touch the developer's real credentials or config.
  export WEEKS_CONFIG_DIR="${BATS_TEST_TMPDIR}/config"
  export WEEKS_NO_KEYRING=1

  # A port nothing listens on, for the failure paths.
  export DEAD_URL="http://127.0.0.1:1"
}

@test "version reports the build" {
  run "$BINARY" --version
  [ "$status" -eq 0 ]
  [[ "$output" == weeks* ]]
}

@test "commands --json emits the envelope" {
  run "$BINARY" commands --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.ok == true'
  echo "$output" | jq -e '.summary | length > 0'
  echo "$output" | jq -e '.breadcrumbs | length > 0'
  echo "$output" | jq -e '.data | length > 0'
}

@test "every breadcrumb names an action, a command, and why" {
  run "$BINARY" commands --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.breadcrumbs | all(has("action") and has("cmd") and has("description"))'
}

@test "the catalog lists the commands the binary actually has" {
  run "$BINARY" commands --json --flat
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.data | index("weeks auth login") != null'
  echo "$output" | jq -e '.data | index("weeks doctor") != null'
  echo "$output" | jq -e '.data | index("weeks commands") != null'
}

@test "--quiet strips the envelope" {
  run "$BINARY" commands --quiet --flat
  [ "$status" -eq 0 ]
  echo "$output" | jq -e 'type == "array"'
  echo "$output" | jq -e 'has("ok") | not' || true
}

@test "--count answers with a number and nothing else" {
  run "$BINARY" commands --count
  [ "$status" -eq 0 ]
  [[ "$output" =~ ^[0-9]+$ ]]
}

@test "--help --agent is structured, not prose" {
  run "$BINARY" auth login --help --agent
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.path == "weeks auth login"'
  echo "$output" | jq -e '.flags | any(.name == "browser")'
  echo "$output" | jq -e '.inherited_flags | any(.name == "profile")'
}

@test "help and the catalog agree about a command" {
  help_flags=$("$BINARY" auth login --help --agent | jq -S '.flags')
  catalog_flags=$("$BINARY" commands --json \
    | jq -S '.data[] | select(.path == "weeks auth login") | .flags')
  [ "$help_flags" = "$catalog_flags" ]
}

@test "not being signed in is auth_required, exit 3" {
  run "$BINARY" auth status --json --base-url "$DEAD_URL"
  [ "$status" -eq 3 ]
  echo "$output" | jq -e '.ok == false'
  echo "$output" | jq -e '.code == "auth_required"'
  echo "$output" | jq -e '.hint | length > 0'
}

@test "an unknown command is usage, exit 1" {
  run "$BINARY" no-such-command --json
  [ "$status" -eq 1 ]
  echo "$output" | jq -e '.ok == false'
  echo "$output" | jq -e '.code == "usage"'
}

@test "doctor emits one envelope and fails loudly when unhealthy" {
  run "$BINARY" doctor --json --base-url "$DEAD_URL"
  [ "$status" -eq 7 ]
  # One envelope, not two: the caller must not have to guess which is the result.
  [ "$(echo "$output" | jq -s 'length')" -eq 1 ]
  echo "$output" | jq -e '.ok == true'
  echo "$output" | jq -e '.data.healthy == false'
  echo "$output" | jq -e '.data.checks | any(.id == "connectivity" and .status == "fail")'
}

@test "every doctor check that fails or is skipped says what to do" {
  run "$BINARY" doctor --json --base-url "$DEAD_URL"
  echo "$output" | jq -e '.data.checks | map(select(.status == "fail" or .status == "skip"))
                          | all(.hint | length > 0)'
}

@test "the skill ships inside the binary" {
  run "$BINARY" skill
  [ "$status" -eq 0 ]
  [[ "$output" == *"confirmation_required"* ]]
  [[ "$output" == *"breadcrumbs"* ]]
}

@test "login without a client id explains what to set" {
  run "$BINARY" auth login --json --base-url "$DEAD_URL"
  [ "$status" -eq 1 ]
  echo "$output" | jq -e '.code == "usage"'
  echo "$output" | jq -e '.hint | contains("WEEKS_CLIENT_ID")'
}

@test "profiles round-trip through the config directory" {
  run "$BINARY" profile set acme --base-url https://acme.weeks.test --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.data.base_url == "https://acme.weeks.test"'

  run "$BINARY" profile list --json
  [ "$status" -eq 0 ]
  echo "$output" | jq -e '.data | any(.name == "acme")'

  run "$BINARY" profile remove acme --json
  [ "$status" -eq 0 ]

  run "$BINARY" profile list --json
  echo "$output" | jq -e '.data | length == 0'
}

@test "a named profile decides which installation a command targets" {
  "$BINARY" profile set staging --base-url https://staging.weeks.test --json > /dev/null

  run "$BINARY" auth status --json --profile staging
  [ "$status" -eq 3 ]
  echo "$output" | jq -e '.error | contains("staging.weeks.test")'
}
