package actions

import (
	"fmt"
	"gosrc/logger"
	"gosrc/parser"

	"os"
)

func CreateServicefile(cfg *parser.Config, binaryPath string) error {
	// 1. Define your extra variables
	execPath := binaryPath //+ " " + cfg.String()

	// 2. Wrap everything in a map
	data := map[string]any{
		"Cfg":     cfg,
		"command": execPath,
	}
	logger.Info("[Creating Systemd Service]", cfg)
	content := `[Unit]
Description=CICD Orchestrator Manager
After=network.target
[Service]
ExecStart={{.command}}
# The manager stays in its own directory
WorkingDirectory=/etc/cicd/
User=root
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
`

	content, err := parser.ParseTemplate(content, data)
	if err != nil {
		logger.Error("[Failed to parse servicefile]", cfg)
		return err
	} else {
		logger.Info("[Servicefile parsed]", cfg)
	}
	fmt.Println(content)

	//Write to file at /etc/systemd/system/{{service_name}}.service
	err = os.WriteFile("/etc/systemd/system/cicd.service", []byte(content), 0644)
	if err != nil {
		logger.Error("[Failed to write servicefile]", cfg)
		return err
	} else {
		logger.Info("[Servicefile written]", cfg)
	}
	return nil
}
