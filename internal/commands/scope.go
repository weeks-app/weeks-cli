package commands

import (
	"strings"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
)

func scopedCommand(app *appctx.App, command string) string {
	if app.ConfigScope == config.ScopeGlobal {
		if command == "weeks" {
			return "weeks --global"
		}
		if strings.HasPrefix(command, "weeks ") {
			return "weeks --global " + strings.TrimPrefix(command, "weeks ")
		}
		return command + " --global"
	}
	return command
}

func profileCommand(app *appctx.App, command, profile string) string {
	if profile != "" {
		command += " --profile " + profile
	}
	return scopedCommand(app, command)
}
