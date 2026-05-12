package actions

import (
	"gosrc/parser"
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func TestSetupProjectFolder(t *testing.T) {
	// 1. Setup a temporary base directory
	tempBase, err := os.MkdirTemp("", "cicd-base-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBase)

	// Get current user for reliable lookup
	currUser, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	// Calculate the expected final path (Simulation of deploypath logic)
	finalPath := filepath.Join(tempBase, "my-test-app-123456")

	// 2. Define a test config
	config := &parser.Config{
		ServiceDir:  finalPath, // The orchestrator expects the resolved final path here
		ServiceName: "my-test-app",
		ServiceUser: currUser.Username,
	}

	t.Run("Directory Creation Logic", func(t *testing.T) {
		// Run setup
		err := SetupProjectFolder(config)
		if err != nil {
			t.Errorf("SetupProjectFolder failed: %v", err)
		}

		// Check if the directory was created
		if _, errStat := os.Stat(finalPath); os.IsNotExist(errStat) {
			t.Errorf("SetupProjectFolder did not create the directory: %s", finalPath)
		}

		// Check if the .cicdlog folder was created inside it
		logFolder := filepath.Join(finalPath, ".cicdlog")
		if _, errStat := os.Stat(logFolder); os.IsNotExist(errStat) {
			t.Errorf("SetupProjectFolder did not create the log directory: %s", logFolder)
		}
	})

	t.Run("Parent Permission Failure Simulation", func(t *testing.T) {
		// Create a 'locked' parent directory
		lockedDir := filepath.Join(tempBase, "locked")
		os.MkdirAll(lockedDir, 0000)
		defer os.Chmod(lockedDir, 0755)

		configLocked := &parser.Config{
			ServiceDir:  filepath.Join(lockedDir, "failed-app"),
			ServiceName: "failed-app",
			ServiceUser: currUser.Username,
		}

		err := SetupProjectFolder(configLocked)
		if err == nil {
			t.Error("Expected an error when parent directory is locked (0000), but got nil")
		} else {
			t.Logf("✅ Successfully caught permission error: %v", err)
		}
	})
}
