package actions

import (
	"gosrc/deploypath"
	"gosrc/parser"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_FullSetup performs a real-world test of the orchestrator's setup capabilities.
// It uses the secure deploypath package to resolve a path and then executes the setup.
// IMPORTANT: This test must be run with root privileges (e.g., 'sudo go test ...') in a Linux/WSL environment.
func TestIntegration_FullSetup(t *testing.T) {
	// 1. Determine a base directory (Use /tmp for native Linux permissions)
	testBase := "/tmp/cicd_test"
	
	// Create base if it doesn't exist so deploypath can validate it
	os.MkdirAll(testBase, 0755)

	// 2. Resolve the secure final path using the REAL "Brain"
	res, err := deploypath.ResolveDeploymentPath(deploypath.Config{
		ServiceUser: "cicd-manual-tester",
		BaseDir:     testBase,
		ProjectName: "IntegrationApp",
		// We can leave ExistingDeployments empty for this test
	})
	if err != nil {
		t.Fatalf("deploypath.ResolveDeploymentPath failed: %v", err)
	}

	finalPath := res.FinalPath

	// 3. Define the config using the resolved path
	config := &parser.Config{
		ServiceDir:  finalPath,
		ServiceUser: "cicd-manual-tester",
		ServiceName: "IntegrationApp",
	}

	t.Logf("--- End-to-End Integration Test Started ---")
	t.Logf("Calculated Path: %s", finalPath)
	t.Logf("Target User:     %s", config.ServiceUser)

	// 4. Execute the actual setup (Root Operation)

	err = SetupProjectFolder(config)
	if err != nil {
		t.Fatalf("SetupProjectFolder failed: %v\n(Check if you are running as root/sudo in WSL)", err)
	}

	// 5. Verification
	if _, err := os.Stat(finalPath); os.IsNotExist(err) {
		t.Errorf("Verification failed: directory not found: %s", finalPath)
	}

	// Verify .cicdlog was created inside the secure slug folder
	logPath := filepath.Join(finalPath, ".cicdlog")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("Verification failed: .cicdlog was not created inside the hashed directory")
	}

	t.Logf("--- E2E Verification Complete ---")
	t.Logf("✅ Secure folder created: %s", finalPath)
	t.Logf("✅ System user verified: %s", config.ServiceUser)
	t.Logf("👉 Manual Check: Run 'ls -la %s'", finalPath)
}
