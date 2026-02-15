package beads

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestGetPrefixForRig(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "gt-", "path": "gastown/mayor/rig"}
{"prefix": "bd-", "path": "beads/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		rig      string
		expected string
	}{
		{"gastown", "gt"},
		{"beads", "bd"},
		{"unknown", "gt"}, // default
		{"", "gt"},        // empty rig -> default
	}

	for _, tc := range tests {
		t.Run(tc.rig, func(t *testing.T) {
			result := GetPrefixForRig(tmpDir, tc.rig)
			if result != tc.expected {
				t.Errorf("GetPrefixForRig(%q, %q) = %q, want %q", tmpDir, tc.rig, result, tc.expected)
			}
		})
	}
}

func TestGetPrefixForRig_NoRoutesFile(t *testing.T) {
	tmpDir := t.TempDir()
	// No routes.jsonl file

	result := GetPrefixForRig(tmpDir, "anything")
	if result != "gt" {
		t.Errorf("Expected default 'gt' when no routes file, got %q", result)
	}
}

func TestGetPrefixForRig_RigsConfigFallback(t *testing.T) {
	tmpDir := t.TempDir()

	// Write rigs.json with a non-gt prefix
	rigsPath := filepath.Join(tmpDir, "mayor", "rigs.json")
	if err := os.MkdirAll(filepath.Dir(rigsPath), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RigsConfig{
		Version: config.CurrentRigsVersion,
		Rigs: map[string]config.RigEntry{
			"project_ideas": {
				BeadsConfig: &config.BeadsConfig{Prefix: "pi"},
			},
		},
	}
	if err := config.SaveRigsConfig(rigsPath, cfg); err != nil {
		t.Fatalf("SaveRigsConfig: %v", err)
	}

	result := GetPrefixForRig(tmpDir, "project_ideas")
	if result != "pi" {
		t.Errorf("Expected prefix from rigs config, got %q", result)
	}
}

func TestExtractPrefix(t *testing.T) {
	tests := []struct {
		beadID   string
		expected string
	}{
		{"ap-qtsup.16", "ap-"},
		{"hq-cv-abc", "hq-"},
		{"gt-mol-xyz", "gt-"},
		{"bd-123", "bd-"},
		{"", ""},
		{"nohyphen", ""},
		{"-startswithhyphen", ""}, // Leading hyphen = invalid prefix
		{"-", ""},                 // Just hyphen = invalid
		{"a-", "a-"},              // Trailing hyphen is valid
	}

	for _, tc := range tests {
		t.Run(tc.beadID, func(t *testing.T) {
			result := ExtractPrefix(tc.beadID)
			if result != tc.expected {
				t.Errorf("ExtractPrefix(%q) = %q, want %q", tc.beadID, result, tc.expected)
			}
		})
	}
}

func TestGetRigPathForPrefix(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ap-", "path": "ai_platform/mayor/rig"}
{"prefix": "gt-", "path": "gastown/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		prefix   string
		expected string
	}{
		{"ap-", filepath.Join(tmpDir, "ai_platform/mayor/rig")},
		{"gt-", filepath.Join(tmpDir, "gastown/mayor/rig")},
		{"hq-", tmpDir},  // Town-level beads return townRoot
		{"unknown-", ""}, // Unknown prefix returns empty
		{"", ""},         // Empty prefix returns empty
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			result := GetRigPathForPrefix(tmpDir, tc.prefix)
			if result != tc.expected {
				t.Errorf("GetRigPathForPrefix(%q, %q) = %q, want %q", tmpDir, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestGetRigPathForPrefix_NoRoutesFile(t *testing.T) {
	tmpDir := t.TempDir()
	// No routes.jsonl file

	result := GetRigPathForPrefix(tmpDir, "ap-")
	if result != "" {
		t.Errorf("Expected empty string when no routes file, got %q", result)
	}
}

func TestResolveHookDir(t *testing.T) {
	// Create a temporary directory with routes.jsonl
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "ap-", "path": "ai_platform/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		beadID      string
		hookWorkDir string
		expected    string
	}{
		{
			name:        "prefix resolution takes precedence over hookWorkDir",
			beadID:      "ap-test",
			hookWorkDir: "/custom/path",
			expected:    filepath.Join(tmpDir, "ai_platform/mayor/rig"),
		},
		{
			name:        "resolves rig path from prefix",
			beadID:      "ap-test",
			hookWorkDir: "",
			expected:    filepath.Join(tmpDir, "ai_platform/mayor/rig"),
		},
		{
			name:        "town-level bead returns townRoot",
			beadID:      "hq-test",
			hookWorkDir: "",
			expected:    tmpDir,
		},
		{
			name:        "unknown prefix uses hookWorkDir as fallback",
			beadID:      "xx-unknown",
			hookWorkDir: "/fallback/path",
			expected:    "/fallback/path",
		},
		{
			name:        "unknown prefix without hookWorkDir falls back to townRoot",
			beadID:      "xx-unknown",
			hookWorkDir: "",
			expected:    tmpDir,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveHookDir(tmpDir, tc.beadID, tc.hookWorkDir)
			if result != tc.expected {
				t.Errorf("ResolveHookDir(%q, %q, %q) = %q, want %q",
					tmpDir, tc.beadID, tc.hookWorkDir, result, tc.expected)
			}
		})
	}
}

func TestGetRigNameForPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "gt-", "path": "gastown/mayor/rig"}
{"prefix": "bd-", "path": "beads/mayor/rig"}
{"prefix": "hq-", "path": "."}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		prefix   string
		expected string
	}{
		{"gt-", "gastown"},
		{"bd-", "beads"},
		{"hq-", ""},       // Town-level, no specific rig
		{"unknown-", ""},  // Not in routes
		{"", ""},          // Empty prefix
	}

	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			result := GetRigNameForPrefix(tmpDir, tc.prefix)
			if result != tc.expected {
				t.Errorf("GetRigNameForPrefix(%q, %q) = %q, want %q", tmpDir, tc.prefix, result, tc.expected)
			}
		})
	}
}

func TestMatchLongestPrefix(t *testing.T) {
	routes := []Route{
		{Prefix: "hq-", Path: "."},
		{Prefix: "hq-wisp-", Path: "wisp-storage"},
		{Prefix: "gt-", Path: "gastown/mayor/rig"},
		{Prefix: "ap-", Path: "ai_platform/mayor/rig"},
	}

	tests := []struct {
		name     string
		beadID   string
		expected string // expected prefix, or "" for nil
	}{
		{"simple prefix", "gt-abc123", "gt-"},
		{"simple prefix 2", "ap-xyz", "ap-"},
		{"short prefix wins for simple ID", "hq-abc", "hq-"},
		{"longest prefix wins for wisp ID", "hq-wisp-u896p", "hq-wisp-"},
		{"longest prefix wins with more segments", "hq-wisp-something-deep", "hq-wisp-"},
		{"no match", "xx-unknown", ""},
		{"empty bead ID", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := matchLongestPrefix(routes, tc.beadID)
			if tc.expected == "" {
				if result != nil {
					t.Errorf("matchLongestPrefix(%q) = %q, want nil", tc.beadID, result.Prefix)
				}
			} else {
				if result == nil {
					t.Errorf("matchLongestPrefix(%q) = nil, want %q", tc.beadID, tc.expected)
				} else if result.Prefix != tc.expected {
					t.Errorf("matchLongestPrefix(%q) = %q, want %q", tc.beadID, result.Prefix, tc.expected)
				}
			}
		})
	}
}

func TestGetRigPathForBeadID(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "hq-", "path": "."}
{"prefix": "hq-wisp-", "path": "wisp-storage"}
{"prefix": "gt-", "path": "gastown/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		{"simple hq bead routes to town", "hq-abc123", tmpDir},
		{"wisp bead routes to wisp-storage", "hq-wisp-u896p", filepath.Join(tmpDir, "wisp-storage")},
		{"gt bead routes to gastown", "gt-mol-xyz", filepath.Join(tmpDir, "gastown/mayor/rig")},
		{"unknown prefix returns empty", "xx-unknown", ""},
		{"empty bead ID returns empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetRigPathForBeadID(tmpDir, tc.beadID)
			if result != tc.expected {
				t.Errorf("GetRigPathForBeadID(%q, %q) = %q, want %q", tmpDir, tc.beadID, result, tc.expected)
			}
		})
	}
}

func TestGetRigNameForBeadID(t *testing.T) {
	tmpDir := t.TempDir()
	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	routesContent := `{"prefix": "hq-", "path": "."}
{"prefix": "hq-wisp-", "path": "wisp-storage/mayor/rig"}
{"prefix": "gt-", "path": "gastown/mayor/rig"}
`
	if err := os.WriteFile(filepath.Join(beadsDir, "routes.jsonl"), []byte(routesContent), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		beadID   string
		expected string
	}{
		{"hq bead is town-level", "hq-abc", ""},
		{"wisp bead routes to wisp-storage rig", "hq-wisp-xyz", "wisp-storage"},
		{"gt bead routes to gastown", "gt-mol-abc", "gastown"},
		{"unknown prefix returns empty", "xx-abc", ""},
		{"empty bead ID returns empty", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GetRigNameForBeadID(tmpDir, tc.beadID)
			if result != tc.expected {
				t.Errorf("GetRigNameForBeadID(%q, %q) = %q, want %q", tmpDir, tc.beadID, result, tc.expected)
			}
		})
	}
}

func TestAgentBeadIDsWithPrefix(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() string
		expected string
	}{
		{"PolecatBeadIDWithPrefix bd beads obsidian",
			func() string { return PolecatBeadIDWithPrefix("bd", "beads", "obsidian") },
			"bd-beads-polecat-obsidian"},
		{"PolecatBeadIDWithPrefix gt gastown Toast",
			func() string { return PolecatBeadIDWithPrefix("gt", "gastown", "Toast") },
			"gt-gastown-polecat-Toast"},
		{"WitnessBeadIDWithPrefix bd beads",
			func() string { return WitnessBeadIDWithPrefix("bd", "beads") },
			"bd-beads-witness"},
		{"RefineryBeadIDWithPrefix bd beads",
			func() string { return RefineryBeadIDWithPrefix("bd", "beads") },
			"bd-beads-refinery"},
		{"CrewBeadIDWithPrefix bd beads max",
			func() string { return CrewBeadIDWithPrefix("bd", "beads", "max") },
			"bd-beads-crew-max"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.fn()
			if result != tc.expected {
				t.Errorf("got %q, want %q", result, tc.expected)
			}
		})
	}
}
