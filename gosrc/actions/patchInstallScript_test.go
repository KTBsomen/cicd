package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectSafetyFlags(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantInjected bool
		contains     []string
	}{
		{
			name: "No shebang, no flags",
			input: `echo "hello"
apt update`,
			wantInjected: true,
			contains:     []string{"set -eo pipefail"},
		},
		{
			name: "With shebang, no flags",
			input: `#!/bin/bash
echo "hello"`,
			wantInjected: true,
			contains:     []string{"#!/bin/bash", "set -eo pipefail"},
		},
		{
			name: "Already has set -e, missing pipefail",
			input: `#!/bin/bash
set -e
echo "hello"`,
			wantInjected: true,
			contains:     []string{"# --- injected by CICD"},
		},
		{
			name: "Already has all flags",
			input: `#!/bin/bash
set -e
set -o pipefail
echo "hello"`,
			wantInjected: false,
		},
		{
			name: "Already has set -xe",
			input: `#!/bin/bash
set -xe
set -o pipefail`,
			wantInjected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := injectSafetyFlags(tt.input, "install.sh")
			if changed != tt.wantInjected {
				t.Errorf("injectSafetyFlags() changed = %v, want %v", changed, tt.wantInjected)
			}
			if tt.wantInjected {
				for _, c := range tt.contains {
					if !strings.Contains(got, c) {
						t.Errorf("expected output to contain %q, but got:\n%s", c, got)
					}
				}
			} else {
				if got != tt.input {
					t.Errorf("expected no change, but got:\n%s", got)
				}
			}
		})
	}
}

func TestNormalizeSudoAFlags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		changed  bool
	}{
		{
			name:     "No sudo",
			input:    "apt update",
			expected: "apt update",
			changed:  false,
		},
		{
			name:     "Already correct",
			input:    "sudo -A apt update",
			expected: "sudo -A apt update",
			changed:  false,
		},
		{
			name:     "Add -A missing completely",
			input:    "sudo apt update",
			expected: "sudo -A apt update",
			changed:  true,
		},
		{
			name:     "Move -A from end",
			input:    "sudo apt update -A",
			expected: "sudo -A apt update",
			changed:  true,
		},
		{
			name:     "Move -A from middle",
			input:    "sudo apt -A install -y",
			expected: "sudo -A apt install -y",
			changed:  true,
		},
		{
			name: "Multiple lines mixed",
			input: `#!/bin/bash
sudo apt update
# sudo comment
sudo -A apt install -y curl
sudo apt install -y git -A`,
			expected: `#!/bin/bash
sudo -A apt update
# sudo comment
sudo -A apt install -y curl
sudo -A apt install -y git`,
			changed: true,
		},
		{
			name:     "Just sudo",
			input:    "sudo",
			expected: "sudo -A",
			changed:  true,
		},
		{
			name:     "Sudo with leading whitespace",
			input:    "   sudo apt update",
			expected: "   sudo -A apt update",
			changed:  true,
		},
		{
			name:     "Comment starting with sudo is ignored",
			input:    "#sudo apt update",
			expected: "#sudo apt update",
			changed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := normalizeSudoAFlags(tt.input)
			if changed != tt.changed {
				t.Errorf("normalizeSudoAFlags() changed = %v, want %v", changed, tt.changed)
			}
			if got != tt.expected {
				t.Errorf("normalizeSudoAFlags() got:\n%q\nwant:\n%q", got, tt.expected)
			}
		})
	}
}

func TestPatchShellScriptIntegration(t *testing.T) {
	tempDir := t.TempDir()
	srcFile := filepath.Join(tempDir, "install.sh")
	dstFile := filepath.Join(tempDir, "install_patched.sh")

	inputContent := `#!/bin/bash
sudo apt update
sudo apt install -y curl -A
`
	if err := os.WriteFile(srcFile, []byte(inputContent), 0755); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	resPath, err := patchShellScript(srcFile, dstFile)
	if err != nil {
		t.Fatalf("unexpected error from patchShellScript: %v", err)
	}

	if resPath != dstFile {
		t.Errorf("expected returned path to be %q, got %q", dstFile, resPath)
	}

	patchedBytes, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read patched file: %v", err)
	}
	patchedContent := string(patchedBytes)

	// Check flags
	if !strings.Contains(patchedContent, "set -eo pipefail") {
		t.Errorf("patched content missing safety flags:\n%s", patchedContent)
	}

	// Check sudo corrections
	if !strings.Contains(patchedContent, "sudo -A apt update") || !strings.Contains(patchedContent, "sudo -A apt install -y curl") {
		t.Errorf("patched content missing normalized sudo commands:\n%s", patchedContent)
	}
}
