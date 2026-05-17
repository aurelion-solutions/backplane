// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// appsSubdir is the conventional layout for per-application cartridges
// inside a bundle: <bundle>/apps/<app_id>/.
const appsSubdir = "apps"

// File names inside an app cartridge directory.
const (
	appManifestFile   = "manifest.yaml"
	appAccountFile    = "account.yaml"
	appDescriptorFile = "descriptor.yaml"
)

// ErrInvalidApp is returned when an app cartridge under apps/<id>/
// fails to parse or violates the contract (missing required fields,
// transitions reference unknown states, descriptor fields mix shapes,
// …).
var ErrInvalidApp = errors.New("cartridges: invalid app")

// AppCartridge is the in-memory projection of one
// <bundle>/apps/<app_id>/ directory.
type AppCartridge struct {
	ID         string              `json:"id"`
	Manifest   AppManifest         `json:"manifest"`
	Account    AccountStateMachine `json:"account"`
	Descriptor Descriptor          `json:"descriptor"`
	// BasePath is the absolute path of the app cartridge directory.
	// Populated by the provider; not a user-authored field.
	BasePath string `json:"-"`
}

// AppManifest mirrors manifest.yaml.
//
// Config holds values that surface in descriptor templates as
// `.Application.<key>`. A nil entry means "no default — must be set
// by the installer"; the loader preserves the absence and does not
// enforce installation.
type AppManifest struct {
	ID        string         `yaml:"id"        json:"id"`
	Name      string         `yaml:"name"      json:"name"`
	Version   string         `yaml:"version"   json:"version"`
	Connector string         `yaml:"connector" json:"connector"`
	Config    map[string]any `yaml:"config"    json:"config,omitempty"`
}

// AccountStateMachine mirrors account.yaml.
type AccountStateMachine struct {
	States       []string            `yaml:"states"        json:"states"`
	InitialState string              `yaml:"initial_state" json:"initial_state"`
	Transitions  []AccountTransition `yaml:"transitions"   json:"transitions"`
}

// AccountTransition is one (from, to) pair allowed by the state
// machine.
type AccountTransition struct {
	From string `yaml:"from" json:"from"`
	To   string `yaml:"to"   json:"to"`
}

// Descriptor mirrors descriptor.yaml.
type Descriptor struct {
	Fields map[string]DescriptorField `yaml:"fields" json:"fields"`
}

// DescriptorField is one entry in descriptor.yaml `fields:` map.
//
// A field is either template-based (Template + optional Transforms /
// OnCollision) or state-keyed (ByState). The two are mutually
// exclusive at the top level — the loader rejects fields that set
// both. By_state values are themselves templates and are resolved by
// the renderer the same way as Template.
type DescriptorField struct {
	Template    string         `yaml:"template,omitempty"     json:"template,omitempty"`
	Transforms  []string       `yaml:"transforms,omitempty"   json:"transforms,omitempty"`
	OnCollision string         `yaml:"on_collision,omitempty" json:"on_collision,omitempty"`
	ByState     map[string]any `yaml:"by_state,omitempty"     json:"by_state,omitempty"`
}

// loadAppCartridge reads the three YAML files from dir, validates
// the result and returns the AppCartridge.
func loadAppCartridge(dir string) (AppCartridge, error) {
	app := AppCartridge{BasePath: dir}

	if err := parseYAMLFile(filepath.Join(dir, appManifestFile), &app.Manifest); err != nil {
		return AppCartridge{}, err
	}
	if err := parseYAMLFile(filepath.Join(dir, appAccountFile), &app.Account); err != nil {
		return AppCartridge{}, err
	}
	if err := parseYAMLFile(filepath.Join(dir, appDescriptorFile), &app.Descriptor); err != nil {
		return AppCartridge{}, err
	}

	app.ID = app.Manifest.ID
	if err := validateAppCartridge(app); err != nil {
		return AppCartridge{}, err
	}
	return app, nil
}

// parseYAMLFile reads path and decodes it into out. Both read and
// parse failures are wrapped with ErrInvalidApp.
func parseYAMLFile(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%w: read %q: %v", ErrInvalidApp, path, err)
	}
	if err := yaml.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: parse %q: %v", ErrInvalidApp, path, err)
	}
	return nil
}

// validateAppCartridge enforces the cross-file contract:
//
//   - manifest.id and manifest.connector are non-empty
//   - account.states is non-empty
//   - account.initial_state is one of account.states
//   - every transition references known states
//   - every descriptor field is either template-shaped or by_state-shaped
//   - every by_state key references a known account state
func validateAppCartridge(app AppCartridge) error {
	if app.Manifest.ID == "" {
		return fmt.Errorf("%w: %s: manifest.id is required", ErrInvalidApp, app.BasePath)
	}
	if app.Manifest.Connector == "" {
		return fmt.Errorf("%w: %s: manifest.connector is required", ErrInvalidApp, app.BasePath)
	}
	if len(app.Account.States) == 0 {
		return fmt.Errorf("%w: %s: account.states is required", ErrInvalidApp, app.BasePath)
	}

	states := make(map[string]struct{}, len(app.Account.States))
	for _, s := range app.Account.States {
		states[s] = struct{}{}
	}
	if _, ok := states[app.Account.InitialState]; !ok {
		return fmt.Errorf("%w: %s: account.initial_state %q not in states",
			ErrInvalidApp, app.BasePath, app.Account.InitialState)
	}
	for i, t := range app.Account.Transitions {
		if _, ok := states[t.From]; !ok {
			return fmt.Errorf("%w: %s: transitions[%d].from %q not in states",
				ErrInvalidApp, app.BasePath, i, t.From)
		}
		if _, ok := states[t.To]; !ok {
			return fmt.Errorf("%w: %s: transitions[%d].to %q not in states",
				ErrInvalidApp, app.BasePath, i, t.To)
		}
	}
	for name, f := range app.Descriptor.Fields {
		hasTemplate := f.Template != ""
		hasByState := len(f.ByState) > 0
		if !hasTemplate && !hasByState {
			return fmt.Errorf("%w: %s: descriptor field %q has neither template nor by_state",
				ErrInvalidApp, app.BasePath, name)
		}
		if hasTemplate && hasByState {
			return fmt.Errorf("%w: %s: descriptor field %q has both template and by_state",
				ErrInvalidApp, app.BasePath, name)
		}
		for s := range f.ByState {
			if _, ok := states[s]; !ok {
				return fmt.Errorf("%w: %s: descriptor field %q references unknown state %q",
					ErrInvalidApp, app.BasePath, name, s)
			}
		}
	}
	return nil
}
