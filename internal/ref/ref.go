// Package ref parses typed weeks API references.
package ref

import (
	"fmt"
	"regexp"
	"strings"
)

// Kind identifies the resource type named by a reference prefix.
type Kind string

const (
	Space                Kind = "space"
	Plan                 Kind = "plan"
	Slot                 Kind = "slot"
	Job                  Kind = "job"
	Inbox                Kind = "inbox"
	Person               Kind = "person"
	PlanPerson           Kind = "plan_person"
	AssignedPerson       Kind = "assigned_person"
	AssignedJob          Kind = "assigned_job"
	AssignedJobPerson    Kind = "job_person"
	JobRoute             Kind = "job_route"
	PlanningContext      Kind = "planning_category"
	PlanningContextValue Kind = "planning_value"
	PlanningState        Kind = "planning_state"
)

var tokenPattern = regexp.MustCompile(`^[A-Za-z]+$`)

// Reference is a type prefix plus the server's obfuscated id token.
type Reference struct {
	Kind  Kind
	Token string
}

func (r Reference) String() string {
	if r.Kind == "" || r.Token == "" {
		return ""
	}

	return string(r.Kind) + "_" + r.Token
}

// Parse validates a typed reference such as "slot_WJmmMJ".
func Parse(raw string) (Reference, error) {
	raw = strings.TrimSpace(raw)
	prefix, token, ok := strings.Cut(raw, "_")
	if !ok {
		return Reference{}, fmt.Errorf("unknown reference type in %q", raw)
	}

	if lastSeparator := strings.LastIndex(raw, "_"); lastSeparator >= 0 {
		prefix = raw[:lastSeparator]
		token = raw[lastSeparator+1:]
	}

	kind, ok := prefixToKind[prefix]
	if !ok {
		return Reference{}, fmt.Errorf("unknown reference type in %q", raw)
	}

	if !tokenPattern.MatchString(token) {
		return Reference{}, fmt.Errorf("invalid %s reference %q", kind, raw)
	}

	return Reference{Kind: kind, Token: token}, nil
}

// ParseKind validates that raw names the expected resource type.
func ParseKind(raw string, expected Kind) (Reference, error) {
	parsed, err := Parse(raw)
	if err != nil {
		return Reference{}, err
	}

	if parsed.Kind != expected {
		return Reference{}, fmt.Errorf("expected %s reference, got %s", expected, parsed.Kind)
	}

	return parsed, nil
}

// TokenFor returns the obfuscated id token after confirming the expected type.
func TokenFor(raw string, expected Kind) (string, error) {
	parsed, err := ParseKind(raw, expected)
	if err != nil {
		return "", err
	}

	return parsed.Token, nil
}

var prefixToKind = map[string]Kind{
	"space":             Space,
	"spc":               Space,
	"plan":              Plan,
	"pln":               Plan,
	"slot":              Slot,
	"slt":               Slot,
	"job":               Job,
	"inbox":             Inbox,
	"inb":               Inbox,
	"person":            Person,
	"per":               Person,
	"plan_person":       PlanPerson,
	"ppn":               PlanPerson,
	"assigned_person":   AssignedPerson,
	"asp":               AssignedPerson,
	"assigned_job":      AssignedJob,
	"asj":               AssignedJob,
	"job_person":        AssignedJobPerson,
	"ajp":               AssignedJobPerson,
	"job_route":         JobRoute,
	"jrt":               JobRoute,
	"planning_category": PlanningContext,
	"plc":               PlanningContext,
	"planning_value":    PlanningContextValue,
	"plv":               PlanningContextValue,
	"planning_state":    PlanningState,
	"pls":               PlanningState,
}
