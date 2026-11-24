// SPDX-License-Identifier: Apache-2.0

package kit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const manifestRoot = "../../agent_tools"

type manifestFile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
		Additional *bool                      `json:"additionalProperties"`
	} `json:"parameters"`
	Command   []string `json:"command"`
	Input     string   `json:"input"`
	Output    string   `json:"output"`
	Timeout   int      `json:"timeout_seconds"`
	Platforms []string `json:"platforms"`
}

func jsonFields(t reflect.Type) []string {
	var out []string
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			out = append(out, tag)
		}
	}
	sort.Strings(out)
	return out
}

// Every registered tool has a manifest whose parameters match the Go request
// type exactly, and every manifest belongs to a registered tool.
func TestManifestsMatchCode(t *testing.T) {
	dirs, err := os.ReadDir(manifestRoot)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		name := d.Name()
		seen[name] = true
		raw, err := os.ReadFile(filepath.Join(manifestRoot, name, name+".json"))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields() // the engine rejects unknown manifest fields
		var m manifestFile
		if err := dec.Decode(&m); err != nil {
			t.Errorf("%s: invalid manifest: %v", name, err)
			continue
		}
		tool, ok := Get(name)
		if !ok {
			t.Errorf("manifest %s has no registered tool", name)
			continue
		}
		if m.Name != name || m.Parameters.Type != "object" || strings.TrimSpace(m.Description) == "" {
			t.Errorf("%s: name, parameters.type or description is wrong", name)
		}
		if m.Input != "stdin_json" || m.Output != "json" || len(m.Command) != 2 || m.Command[0] != "{dir}/"+name || m.Command[1] != "--agent-json" {
			t.Errorf("%s: command/input/output do not follow the toolkit convention: %v", name, m.Command)
		}
		if m.Timeout <= 0 || len(m.Platforms) != 3 {
			t.Errorf("%s: timeout_seconds and platforms are required", name)
		}
		if m.Parameters.Additional == nil || *m.Parameters.Additional {
			t.Errorf("%s: additionalProperties must be false", name)
		}
		var props []string
		for k := range m.Parameters.Properties {
			props = append(props, k)
		}
		sort.Strings(props)
		if want := jsonFields(tool.Request); !reflect.DeepEqual(props, want) {
			t.Errorf("%s: manifest parameters %v do not match the request fields %v", name, props, want)
		}
		for _, r := range m.Parameters.Required {
			if _, ok := m.Parameters.Properties[r]; !ok {
				t.Errorf("%s: required parameter %q is not defined", name, r)
			}
		}
		if _, err := os.Stat(filepath.Join(manifestRoot, name, "README.md")); err != nil {
			t.Errorf("%s: README.md is missing", name)
		}
	}
	for _, tool := range All() {
		if !seen[tool.Name] {
			t.Errorf("tool %s has no manifest in %s", tool.Name, manifestRoot)
		}
	}
	if len(All()) != len(seen) {
		t.Errorf("%d tools registered, %d manifests", len(All()), len(seen))
	}
}

// The registry entries point at existing manifests with matching ids.
func TestRegistryEntries(t *testing.T) {
	raw, err := os.ReadFile("../../../../registry.json")
	if err != nil {
		t.Skip("registry.json not found")
	}
	var reg struct {
		Categories []struct{ ID string } `json:"categories"`
		Tools      []struct {
			ID           string            `json:"id"`
			Category     string            `json:"category"`
			Suite        string            `json:"suite"`
			ManifestPath string            `json:"manifest_path"`
			Downloads    map[string]string `json:"downloads"`
			ReleaseTag   string            `json:"release_tag"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &reg); err != nil {
		t.Fatal(err)
	}
	cats := map[string]bool{}
	for _, c := range reg.Categories {
		cats[c.ID] = true
	}
	found := 0
	for _, e := range reg.Tools {
		if e.Suite != "media_toolkit" {
			continue
		}
		found++
		if _, ok := Get(e.ID); !ok {
			t.Errorf("registry lists unknown tool %s", e.ID)
		}
		if !cats[e.Category] {
			t.Errorf("%s: unknown category %s", e.ID, e.Category)
		}
		if _, err := os.Stat(filepath.Join("../../../..", e.ManifestPath)); err != nil {
			t.Errorf("%s: manifest_path %s does not exist", e.ID, e.ManifestPath)
		}
		for p, u := range e.Downloads {
			want := "/" + e.ReleaseTag + "/" + e.ID + "-"
			if !strings.Contains(u, want) || !strings.HasSuffix(u, "-"+p+".zip") {
				t.Errorf("%s: download for %s does not follow the release naming: %s", e.ID, p, u)
			}
		}
	}
	if found != len(All()) {
		t.Errorf("registry has %d media_toolkit tools, code has %d", found, len(All()))
	}
}
