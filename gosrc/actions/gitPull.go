package actions

import (
	"gosrc/logger"
	"gosrc/parser"
)

func GitPull(cfg *parser.Config) error {
	logger.Info("Git Pull", cfg)
	return nil
}
