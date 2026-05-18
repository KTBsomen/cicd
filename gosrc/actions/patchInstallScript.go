package actions

import (
	"fmt"
	"os"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Script Patcher
//  Applies automatic fixes to user install and run scripts before execution.
//  The original file is NEVER modified — all patches go to a temp copy.
// ─────────────────────────────────────────────────────────────────────────────

// patchShellScript applies all available patches to the user's shell script (install.sh or run.sh):
//  1. Injects "set -e" and "set -o pipefail" if not already present, so that
//     command failures cause a non-zero exit instead of being silently ignored.
//  2. Normalizes every sudo invocation to use the -A flag immediately after
//     "sudo", so the SUDO_ASKPASS mechanism works for every sudo call.
//
// Writes the patched script to dstPath. Returns the path that should be
// executed (dstPath on success, srcPath as fallback if anything goes wrong).
func patchShellScript(srcPath, dstPath string) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return srcPath, err
	}

	content := string(data)
	needsPatch := false

	// Determine script type for logging
	scriptName := "script"
	if strings.Contains(srcPath, "install.sh") {
		scriptName = "install.sh"
	} else if strings.Contains(srcPath, "run.sh") {
		scriptName = "run.sh"
	}

	// ── Pass 1: inject set -e / set -o pipefail ──────────────────────────
	content, flagsInjected := injectSafetyFlags(content, scriptName)
	if flagsInjected {
		needsPatch = true
	}

	// ── Pass 2: normalize sudo -A placement ──────────────────────────────
	content, sudoFixed := normalizeSudoAFlags(content)
	if sudoFixed {
		needsPatch = true
	}

	if !needsPatch {
		return srcPath, nil // script is already correct, run original
	}

	if err := os.WriteFile(dstPath, []byte(content), 0755); err != nil {
		return srcPath, err // fallback to original if write fails
	}
	return dstPath, nil
}

// ─────────────────────────────────────────────────────────────────────────────
//  Pass 1 — Safety flag injection
// ─────────────────────────────────────────────────────────────────────────────

// injectSafetyFlags returns the content with "set -e" and "set -o pipefail"
// injected right after the shebang line (or at the top if no shebang).
// The second return value reports whether any change was made.
func injectSafetyFlags(content string, scriptName string) (string, bool) {
	hasSetE := strings.Contains(content, "set -e") ||
		strings.Contains(content, "set -eo") ||
		strings.Contains(content, "set -xe")
	hasPipefail := strings.Contains(content, "pipefail")

	if hasSetE && hasPipefail {
		return content, false // nothing to do
	}

	var inject strings.Builder
	fmt.Fprintf(&inject, "\n# --- injected by CICD (%s): ensure non-zero exit on failure ---\n", scriptName)
	if !hasSetE {
		inject.WriteString("set -Euo pipefail       # exit immediately on any command failure\n")
	}
	// if !hasPipefail {
	// 	inject.WriteString("set -o pipefail # catch failures inside pipes\n")
	// }
	inject.WriteString("# -----------------------------------------------------------\n")

	lines := strings.SplitAfter(content, "\n")

	// Insert after shebang, or at the very top
	insertAt := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		insertAt = 1
	}

	result := make([]string, 0, len(lines)+4)
	result = append(result, lines[:insertAt]...)
	result = append(result, inject.String())
	result = append(result, lines[insertAt:]...)

	return strings.Join(result, ""), true
}

// ─────────────────────────────────────────────────────────────────────────────
//  Pass 2 — sudo -A normalization
// ─────────────────────────────────────────────────────────────────────────────

// normalizeSudoAFlags scans content line by line and ensures every sudo
// invocation has -A placed immediately after "sudo".
//
// Rules:
//
//	"sudo apt update"        →  "sudo -A apt update"   (add -A)
//	"sudo -A apt update"     →  unchanged               (already correct)
//	"sudo apt update -A"     →  "sudo -A apt update"   (move -A)
//	"sudo apt -A install -y" →  "sudo -A apt install -y" (move -A)
//	"# sudo something"       →  unchanged               (comment line)
func normalizeSudoAFlags(content string) (string, bool) {
	lines := strings.Split(content, "\n")
	changed := false

	for i, line := range lines {
		if !isSudoCommandLine(line) {
			continue
		}
		if sudoHasAImmediately(line) {
			continue // already in the right place
		}
		fixed := moveSudoAToFront(line)
		if fixed != line {
			lines[i] = fixed
			changed = true
		}
	}

	return strings.Join(lines, "\n"), changed
}

// isSudoCommandLine reports whether a line contains a sudo invocation as its
// first command token. Comment lines (trimmed starts with #) are excluded.
func isSudoCommandLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	// Must start (after any leading whitespace) with "sudo " or just "sudo"
	return trimmed == "sudo" || strings.HasPrefix(trimmed, "sudo ")
}

// sudoHasAImmediately reports whether -A is already the very first argument
// after "sudo" on the given line.
//
//	"sudo -A apt update"     → true
//	"sudo -A"                → true
//	"sudo apt update"        → false
//	"sudo apt update -A"     → false
func sudoHasAImmediately(line string) bool {
	trimmed := strings.TrimSpace(line)
	// After "sudo", the next non-space token must be "-A"
	rest := strings.TrimPrefix(trimmed, "sudo")
	rest = strings.TrimLeft(rest, " \t")
	return rest == "-A" || strings.HasPrefix(rest, "-A ")
}

// moveSudoAToFront normalizes a sudo line so -A is the first argument.
// It tokenizes everything after "sudo", removes any existing -A tokens,
// then rebuilds as: <leading_whitespace>sudo -A <remaining_args>.
func moveSudoAToFront(line string) string {
	// Split leading whitespace from the rest
	trimmed := strings.TrimSpace(line)
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

	// Everything after "sudo"
	afterSudo := strings.TrimLeft(strings.TrimPrefix(trimmed, "sudo"), " \t")

	// Tokenize and filter out any -A tokens
	tokens := strings.Fields(afterSudo)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t == "-A" {
			continue
		}
		filtered = append(filtered, t)
	}

	if len(filtered) == 0 {
		return leading + "sudo -A"
	}
	return leading + "sudo -A " + strings.Join(filtered, " ")
}
