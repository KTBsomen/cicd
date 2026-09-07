package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathSecurityValidation(t *testing.T) {
	baseDir, err := os.MkdirTemp("", "cicd_path_sec_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp base dir: %v", err)
	}
	defer os.RemoveAll(baseDir)

	// Create some files inside baseDir
	envFile := filepath.Join(baseDir, ".env")
	_ = os.WriteFile(envFile, []byte("PORT=3000"), 0644)

	subDir := filepath.Join(baseDir, "config")
	_ = os.MkdirAll(subDir, 0755)
	configFile := filepath.Join(subDir, "app.json")
	_ = os.WriteFile(configFile, []byte("{}"), 0644)

	tests := []struct {
		name        string
		reqPath     string
		shouldError bool
	}{
		{"Root directory", "", false},
		{"Root dot", ".", false},
		{"Valid env file", ".env", false},
		{"Valid nested file", "config/app.json", false},
		{"Valid nested file with slash", "/config/app.json", false},
		{"Simple traversal attack", "../outside.txt", true},
		{"Deep traversal attack", "../../etc/passwd", true},
		{"Deep traversal windows style", `..\..\..\windows\system32\calc.exe`, true},
		{"Traversal with clean trick", "config/../../etc/shadow", true},
		{"Null byte injection", ".env\x00evil.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, err := resolveAndValidateProjectPath(baseDir, tt.reqPath)
			if tt.shouldError {
				if err == nil {
					t.Errorf("Expected error for '%s', but got resolved path: %s", tt.reqPath, resolved)
				}
			} else {
				if err != nil {
					t.Errorf("Expected success for '%s', but got error: %v", tt.reqPath, err)
				}
				cleanBase, _ := filepath.Abs(baseDir)
				if !strings.HasPrefix(resolved, cleanBase) {
					t.Errorf("Resolved path '%s' is not within base dir '%s'", resolved, cleanBase)
				}
			}
		})
	}
}
