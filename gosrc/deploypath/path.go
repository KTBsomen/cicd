package deploypath

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrPathNotAbsolute    = errors.New("path must be absolute")
	ErrProtectedPath      = errors.New("protected system path")
	ErrPathInsideDeploy   = errors.New("base path is inside existing deployment")
	ErrPathNotDirectory   = errors.New("path is not a directory")
	ErrPathDoesNotExist   = errors.New("path does not exist")
	ErrFinalPathCollision = errors.New("generated deployment path already exists")
	ErrFinalPathEscaped   = errors.New("generated path escaped base directory")
)

type Config struct {
	ServiceUser string
	BaseDir     string
	ProjectName string

	// Existing deployment directories to prevent nesting
	ExistingDeployments []string
}

type Result struct {
	BaseDir        string
	DeploymentName string
	FinalPath      string
}

var protectedPaths = map[string]struct{}{
	"/":      {},
	"/etc":   {},
	"/usr":   {},
	"/bin":   {},
	"/sbin":  {},
	"/lib":   {},
	"/lib64": {},
	"/boot":  {},
	"/dev":   {},
	"/proc":  {},
	"/sys":   {},
	"/run":   {},
}

// ResolveDeploymentPath generates a secure, unique, and collision-free path for a project.
// It focuses on logical validation (collisions, escaping, system paths).
// The actual disk creation and permission setting is handled by the actions package.
func ResolveDeploymentPath(cfg Config) (*Result, error) {
	if cfg.BaseDir == "" {
		return nil, errors.New("base dir required")
	}

	if cfg.ProjectName == "" {
		return nil, errors.New("project name required")
	}

	// Normalize paths to prevent bypasses like /var/www/../www
	baseDir, err := filepath.Abs(cfg.BaseDir)
	if err != nil {
		return nil, err
	}
	baseDir = filepath.ToSlash(filepath.Clean(baseDir))

	// Resolve symlinks to prevent attackers from escaping validations
	resolvedBase, err := filepath.EvalSymlinks(baseDir)
	if err == nil {
		baseDir = filepath.ToSlash(resolvedBase)
	}

	// Absolute path validation
	if !filepath.IsAbs(baseDir) {
		return nil, ErrPathNotAbsolute
	}

	// Protected system paths check
	if _, exists := protectedPaths[baseDir]; exists {
		return nil, ErrProtectedPath
	}

	// Base path must exist and be a directory
	stat, err := os.Stat(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrPathDoesNotExist
		}
		return nil, err
	}
	if !stat.IsDir() {
		return nil, ErrPathNotDirectory
	}

	// Nested deployment protection
	for _, deployment := range cfg.ExistingDeployments {
		deployment = filepath.ToSlash(filepath.Clean(deployment))
		if IsSubpath(baseDir, deployment) || baseDir == deployment {
			return nil, fmt.Errorf("%w: %s inside %s", ErrPathInsideDeploy, baseDir, deployment)
		}
	}

	// Generate clean slug from project name
	slug := Slugify(cfg.ProjectName)

	// Generate random suffix to avoid any collisions
	suffix, err := RandomHex(6)
	if err != nil {
		return nil, err
	}

	deploymentName := fmt.Sprintf("%s-%s", slug, suffix)
	finalPath := filepath.ToSlash(filepath.Join(baseDir, deploymentName))
	finalPath = filepath.ToSlash(filepath.Clean(finalPath))

	// Final safety check: ensure the generated path is still inside the base directory
	if !IsSubpath(finalPath, baseDir) {
		return nil, ErrFinalPathEscaped
	}

	// Collision check in the filesystem
	if _, err := os.Stat(finalPath); err == nil {
		return nil, ErrFinalPathCollision
	}

	return &Result{
		BaseDir:        baseDir,
		DeploymentName: deploymentName,
		FinalPath:      finalPath,
	}, nil
}

// Slugify sanitizes a string for use in filenames
func Slugify(v string) string {
	v = strings.ToLower(v)
	v = strings.TrimSpace(v)

	var out strings.Builder
	lastDash := false

	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			out.WriteRune('-')
			lastDash = true
		}
	}

	s := out.String()
	s = strings.Trim(s, "-")
	if s == "" {
		return "app"
	}
	return s
}

// RandomHex generates a random string of hex characters
func RandomHex(length int) (string, error) {
	buf := make([]byte, length/2+1)
	_, err := rand.Read(buf)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(buf)[:length], nil
}

// IsSubpath checks if a path is a sub-directory of a parent directory
func IsSubpath(path, parent string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	parent = filepath.ToSlash(filepath.Clean(parent))

	if path == parent {
		return true
	}

	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(rel, "..") && rel != ".."
}
