package gateway

import "strings"

// Approval-severity classification for the HITL inline-button approval flow.
// Pure functions (no gateway state) — extracted from telegram.go for testability
// (see approval_test.go). assessSeverity maps a tool call to low/medium/critical;
// assessCommandSeverity classifies a terminal command; isSafeCommand is the
// low-risk predicate.

// assessSeverity returns the risk level of a tool call.
// "low" = auto-approve silently
// "medium" = ask once, then whitelist for session
// "critical" = always ask, never whitelisted
func assessSeverity(toolName, argsJSON string) string {
	switch toolName {
	case "terminal":
		args := kyociParseArgs(argsJSON)
		cmd, _ := args["command"].((string))
		return assessCommandSeverity(cmd)
	case "file":
		args := kyociParseArgs(argsJSON)
		action, _ := args["action"].(string)
		switch action {
		case "read", "list", "search":
			return "low"
		case "write", "mkdir":
			return "medium"
		case "delete":
			return "critical"
		default:
			return "medium"
		}
	case "security_scan":
		return "low"
	case "delegation":
		return "medium"
	default:
		return "low"
	}
}

// assessCommandSeverity classifies a terminal command by risk.
func assessCommandSeverity(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return "low"
	}
	lower := strings.ToLower(cmd)

	// ── CRITICAL: always ask, never whitelisted ──
	criticalPatterns := []string{
		"rm -rf", "rm -fr", "rmdir",
		"kill -9", "killall", "pkill",
		"shutdown", "reboot", "halt",
		"mkfs", "dd if=", "> /dev/sd",
		"chmod 777", "chown -R",
		"git push --force", "git push -f ", "git reset --hard",
		"git clean -fd",
		"docker rm", "docker rmi", "docker system prune",
		"docker volume rm", "docker network rm",
		"kubectl delete", "kubectl drain",
		"sudo ",
		"drop table", "drop database", "truncate ",
		"shred ",
		"iptables -F", "ufw disable",
		"systemctl stop", "systemctl disable",
		"launchctl unload",
	}
	for _, p := range criticalPatterns {
		if strings.Contains(lower, p) {
			return "critical"
		}
	}

	// ── LOW: safe dev/ops commands — auto-approve ──
	base := strings.Fields(cmd)
	if len(base) == 0 {
		return "low"
	}
	baseCmd := base[0]
	if idx := strings.LastIndex(baseCmd, "/"); idx >= 0 {
		baseCmd = baseCmd[idx+1:]
	}

	safeCommands := map[string]bool{
		// read-only inspection
		"ls": true, "cat": true, "head": true, "tail": true, "less": true,
		"pwd": true, "echo": true, "whoami": true, "id": true,
		"date": true, "uptime": true, "uname": true, "hostname": true,
		"df": true, "du": true, "free": true, "vm_stat": true,
		"ps": true, "top": true, "htop": true,
		"grep": true, "rg": true, "find": true, "fd": true,
		"wc": true, "sort": true, "uniq": true, "cut": true, "tr": true,
		"diff": true, "file": true, "stat": true, "touch": true,
		// dev tooling — safe to run
		"git": true, "npm": true, "npx": true, "node": true, "python3": true,
		"python": true, "pip": true, "uv": true, "go": true, "cargo": true,
		"rustc": true, "java": true, "mvn": true, "gradle": true,
		"make": true, "cmake": true,
		// docker — safe read/inspect/build
		"docker": true, "docker-compose": true,
		// network inspection
		"curl": true, "wget": true, "ping": true, "dig": true, "nslookup": true,
		"netstat": true, "ss": true, "lsof": true, "ifconfig": true,
		// system info
		"which": true, "whereis": true, "type": true,
		"env": true, "printenv": true,
		"mdfind": true, "mdls": true,
		"sw_vers": true, "sysctl": true,
		// text tools
		"sed": true, "awk": true, "jq": true, "yq": true,
		"tee": true, "xargs": true,
		"tar": true, "unzip": true, "gzip": true,
	}
	if safeCommands[baseCmd] {
		// Even safe base commands can be dangerous with certain flags
		if baseCmd == "git" && (strings.Contains(lower, "push --force") || strings.Contains(lower, "reset --hard")) {
			return "critical" // already caught above, but double-check
		}
		if baseCmd == "docker" && (strings.Contains(lower, "rm ") || strings.Contains(lower, "rmi ") || strings.Contains(lower, "prune")) {
			return "critical"
		}
		if baseCmd == "curl" && (strings.Contains(lower, "-x post") || strings.Contains(lower, "-x put") || strings.Contains(lower, "-x delete") || strings.Contains(lower, "--request post") || strings.Contains(lower, "--request delete")) {
			return "medium" // API mutations need one-time approval
		}
		return "low"
	}

	// ── MEDIUM: unknown commands — ask once ──
	return "medium"
}

// isSafeCommand is kept for backward compat (isRiskyTool replacement).
func isSafeCommand(cmd string) bool {
	return assessCommandSeverity(cmd) == "low"
}
