package commands

import (
	"encoding/json"
	"fmt"

	bcprofile "github.com/basecamp/cli/profile"

	"github.com/weeks-app/weeks-cli/internal/appctx"
	"github.com/weeks-app/weeks-cli/internal/config"
)

const defaultProfileName = "default"

type defaults struct {
	TeamID  string `json:"team_id,omitempty"`
	SpaceID string `json:"space_id,omitempty"`
}

func currentDefaults(app *appctx.App) defaults {
	if app.Profile == "" {
		return defaults{}
	}
	prof, err := app.Profiles.Get(app.Profile)
	if err != nil {
		return defaults{}
	}
	return profileDefaults(prof)
}

func profileDefaults(prof *bcprofile.Profile) defaults {
	if prof == nil || prof.Extra == nil {
		return defaults{}
	}
	return defaults{
		TeamID:  profileStringExtra(prof, "default_team_id"),
		SpaceID: profileStringExtra(prof, "default_space_id"),
	}
}

func profileStringExtra(prof *bcprofile.Profile, key string) string {
	raw, ok := prof.Extra[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

func setProfileStringExtra(prof *bcprofile.Profile, key, value string) error {
	if prof.Extra == nil {
		prof.Extra = map[string]json.RawMessage{}
	}
	if value == "" {
		delete(prof.Extra, key)
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	prof.Extra[key] = encoded
	return nil
}

func upsertProfile(store *bcprofile.Store, prof *bcprofile.Profile, makeDefault bool) error {
	profiles, defaultName, err := store.List()
	if err != nil {
		return err
	}
	if _, exists := profiles[prof.Name]; exists {
		if err := store.Delete(prof.Name); err != nil {
			return err
		}
	}
	if err := store.Create(prof); err != nil {
		return err
	}
	if makeDefault || defaultName == prof.Name {
		return store.SetDefault(prof.Name)
	}
	return nil
}

func ensureLocalDefaultsProfile(app *appctx.App) (string, error) {
	if app.ConfigScope != config.ScopeLocal {
		return "", fmt.Errorf("defaults are local to a working folder; run without --global or WEEKS_CONFIG_DIR")
	}

	profiles, defaultName, err := app.Profiles.List()
	if err != nil {
		return "", err
	}

	name := app.Profile
	if name == "" {
		name = defaultName
	}
	if name == "" {
		name = defaultProfileName
	}

	prof := profiles[name]
	if prof == nil {
		prof = &bcprofile.Profile{Name: name, BaseURL: app.BaseURL}
	}
	if prof.BaseURL == "" {
		prof.BaseURL = app.BaseURL
	}

	if err := upsertProfile(app.Profiles, prof, true); err != nil {
		return "", err
	}

	if app.Profile == "" {
		creds, err := app.Creds().Load("", app.BaseURL)
		if err == nil {
			if err := app.Creds().Save(name, creds); err != nil {
				return "", err
			}
			_ = app.Creds().Delete("", app.BaseURL)
		}
	}

	return name, nil
}
