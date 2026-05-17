package deploypath_test

import (
	"gosrc/deploypath"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSlugify verifies that project names are correctly sanitized into filesystem-safe slugs.
func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"myapp", "myapp"},
		{"My App", "my-app"},
		{"web-api", "web-api"},
		{"Web_API v2!", "web-api-v2"},
		{"   spaces   ", "spaces"},
		{"!!!###", "app"}, // all special chars → fallback "app"
		{"MyApp123", "myapp123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := deploypath.Slugify(tt.input)
			if got != tt.expected {
				t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestResolveDeploymentPath verifies path resolution using real temp directories.
func TestResolveDeploymentPath(t *testing.T) {
	// Use a real temp dir so os.Stat checks pass
	base := t.TempDir()

	tests := []struct {
		name        string
		projectName string
		wantErr     bool
	}{
		{
			name:        "Valid project name",
			projectName: "my-api",
			wantErr:     false,
		},
		{
			name:        "Project with spaces (slugified)",
			projectName: "My Cool App",
			wantErr:     false,
		},
		{
			name:        "Empty project name",
			projectName: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := deploypath.ResolveDeploymentPath(deploypath.Config{
				ServiceUser: "testuser",
				BaseDir:     base,
				ProjectName: tt.projectName,
			})

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// The final path must be inside the base directory
			rel, err := filepath.Rel(base, res.FinalPath)
			if err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("FinalPath %q escaped base dir %q", res.FinalPath, base)
			}

			// The final path must NOT already exist (no collision)
			if _, statErr := os.Stat(res.FinalPath); statErr == nil {
				t.Errorf("FinalPath %q already exists — collision not prevented", res.FinalPath)
			}
		})
	}
}

// TestProtectedPaths ensures system directories are rejected.
func TestProtectedPaths(t *testing.T) {
	protected := []string{"/", "/etc", "/usr", "/boot"}
	for _, p := range protected {
		t.Run(p, func(t *testing.T) {
			_, err := deploypath.ResolveDeploymentPath(deploypath.Config{
				ServiceUser: "testuser",
				BaseDir:     p,
				ProjectName: "myapp",
			})
			if err == nil {
				t.Errorf("expected error for protected path %q, got nil", p)
			}
		})
	}
}

// TestIsSubpath verifies that path containment checks work correctly.
func TestIsSubpath(t *testing.T) {
	tests := []struct {
		path     string
		parent   string
		expected bool
	}{
		{"/home/user/myapp", "/home/user", true},
		{"/home/user", "/home/user", true},
		{"/home/other", "/home/user", false},
		{"/home/user/../etc", "/home/user", false},
		{"/var/www/project", "/home", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := deploypath.IsSubpath(tt.path, tt.parent)
			if got != tt.expected {
				t.Errorf("IsSubpath(%q, %q) = %v, want %v", tt.path, tt.parent, got, tt.expected)
			}
		})
	}
}
