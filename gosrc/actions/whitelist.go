package actions

import "strings"

var whitelist = []string{
	"/usr/bin/apt",
	"/usr/bin/apt-get",
	"/usr/bin/systemctl",
	"/usr/bin/service",
	"/usr/bin/dnf",
	"/usr/bin/zypper",
	"/usr/bin/pacman",
	"/usr/bin/yum",
	"/usr/bin/git",
	"/usr/bin/curl",
	"/usr/bin/wget",
	"/usr/bin/unzip",
	"/usr/bin/tar",
	"/usr/bin/node",
	"/usr/bin/npm",
	"/usr/bin/npx",
	"/usr/bin/tee",
	"/usr/bin/mkdir",
	"/usr/bin/cp",
	"/usr/bin/mv",
	"/usr/bin/chmod",
	"/usr/bin/chown",
	"/usr/sbin/caddy",
	"/usr/bin/caddy",
	"/usr/bin/pm2",
	"/usr/bin/redis-server",
	"/usr/bin/redis-cli",
}

// GetWhiteListForSUDO returns a comma-separated string of commands that the service user is allowed to run with sudo.
func GetWhiteListForSUDO() string {
	return strings.Join(whitelist, ",")
}
