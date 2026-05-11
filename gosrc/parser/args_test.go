package parser

import "testing"

func TestGetProjectPath(t *testing.T) {
	tests := []struct {
		name        string
		user        string
		serviceName string
		serviceDir  string
		expected    string
	}{
		{
			name:        "Default Home Scenario",
			user:        "ubuntu",
			serviceName: "myapp",
			serviceDir:  "/home/",
			expected:    "/home/ubuntu/myapp",
		},
		{
			name:        "Root User Scenario",
			user:        "root",
			serviceName: "sys-tool",
			serviceDir:  "/home/",
			expected:    "/root/sys-tool",
		},
		{
			name:        "Custom System Directory",
			user:        "ubuntu",
			serviceName: "web-api",
			serviceDir:  "/var/www",
			expected:    "/var/www/web-api",
		},
		{
			name:        "Trailing Slash Cleanup",
			user:        "somen",
			serviceName: "app",
			serviceDir:  "/opt/custom///",
			expected:    "/opt/custom/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				ServiceUser: tt.user,
				ServiceName: tt.serviceName,
				ServiceDir:  tt.serviceDir,
			}
			if got := c.GetProjectPath(); got != tt.expected {
				t.Errorf("GetProjectPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}
