// Package skills embeds the agent skill in the binary.
//
// Shipping the skill inside the CLI is the point: a binary that can print the
// document that teaches it needs no separate distribution, and can never ship
// a skill that describes a different version of itself.
package skills

import "embed"

//go:embed weeks
var FS embed.FS
