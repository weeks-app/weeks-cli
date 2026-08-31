package ref_test

import (
	"strings"
	"testing"

	"github.com/weeks-app/weeks-cli/internal/ref"
)

func TestParse(t *testing.T) {
	parsed, err := ref.Parse("assigned_job_WJmmMJ")
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Kind != ref.AssignedJob {
		t.Fatalf("kind = %q, want %q", parsed.Kind, ref.AssignedJob)
	}

	if parsed.Token != "WJmmMJ" {
		t.Fatalf("token = %q, want WJmmMJ", parsed.Token)
	}

	if parsed.String() != "assigned_job_WJmmMJ" {
		t.Fatalf("String() = %q", parsed.String())
	}
}

func TestParseAcceptsAliasAndNormalizesOutput(t *testing.T) {
	parsed, err := ref.Parse("asj_WJmmMJ")
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Kind != ref.AssignedJob {
		t.Fatalf("kind = %q, want %q", parsed.Kind, ref.AssignedJob)
	}

	if parsed.String() != "assigned_job_WJmmMJ" {
		t.Fatalf("String() = %q", parsed.String())
	}
}

func TestParseRejectsWrongKind(t *testing.T) {
	if _, err := ref.ParseKind("job_WJmmMJ", ref.AssignedJob); err == nil {
		t.Fatal("ParseKind accepted a job reference where an assigned job was required")
	}
}

func TestParseRejectsBareIDs(t *testing.T) {
	for _, raw := range []string{"42", "WJmmMJ", "asj_42", "asj_WJmmMJ_", "assigned_job_WJmmMJ_"} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ref.Parse(raw); err == nil {
				t.Fatalf("Parse(%q) succeeded, want failure", raw)
			}
		})
	}
}

func TestLongerPrefixesWin(t *testing.T) {
	parsed, err := ref.Parse("planning_value_xQJmZa")
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Kind != ref.PlanningContextValue {
		t.Fatalf("kind = %q, want %q", parsed.Kind, ref.PlanningContextValue)
	}
}

func TestCurrentPrefixesAreReadable(t *testing.T) {
	for _, raw := range []ref.Kind{
		ref.Space,
		ref.Plan,
		ref.Slot,
		ref.Job,
		ref.Inbox,
		ref.Person,
		ref.PlanPerson,
		ref.AssignedPerson,
		ref.AssignedJob,
		ref.AssignedJobPerson,
		ref.JobRoute,
		ref.PlanningContext,
		ref.PlanningContextValue,
		ref.PlanningState,
	} {
		if strings.Trim(string(raw), "abcdefghijklmnopqrstuvwxyz_") != "" {
			t.Fatalf("kind %q contains unsupported characters", raw)
		}
	}
}
