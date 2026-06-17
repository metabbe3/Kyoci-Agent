package orchestrator

import (
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ClassifyRole auto-detects which role should handle a task based on heuristic keywords.
// Priority order (most specific first, most general last):
//   1. Frontend — UI/HTML/CSS/React/Vue
//   2. SRE — system/infra/monitoring/server
//   3. QA — testing/review/security
//   4. PM — planning/timeline/project
//   5. Developer — general coding (default fallback)
//
// This is a lightweight classifier that doesn't require LLM calls.
func ClassifyRole(task string) kyoci.RoleType {
	taskLower := strings.ToLower(task)

	// ── 1. Frontend — UI/HTML/CSS/React ──────────────────────────────
	frontendKeywords := []string{
		"html", "css", "scss", "tailwind", "frontend", "ui ", "ux ",
		"component", "button", "navbar", "sidebar", "footer", "landing page",
		"responsive", "flexbox", "grid layout", "media query",
		"react", "next.js", "nextjs", "vue", "svelte", "astro",
		"dom", "accessibility", "aria", "typescript", ".tsx", ".jsx",
		"web page", "webpage", "website design",
	}
	for _, keyword := range frontendKeywords {
		if strings.Contains(taskLower, keyword) {
			return kyoci.RoleFrontend
		}
	}

	// ── 2. SRE — system/infra/monitoring ─────────────────────────────
	// Checked BEFORE developer because "check disk", "check cpu" would match dev keyword "check".
	sreKeywords := []string{
		// System resources
		"disk space", "disk usage", "cpu ", "memory usage", "ram ",
		"system performance", "machine performance", "server load",
		"uptime", "health check", "health status",
		// Infrastructure
		"deploy", "docker", "kubernetes", "k8s", "container",
		"nginx", "load balanc", "scaling", "autoscal",
		"monitor", "alert", "incident", "outage",
		"infra", "ops ", "production server", "staging",
		// Network
		"port", "firewall", "dns", "ssl", "certificate",
		"network", "connection", "ping ", "latency",
		// Log/metrics
		"log file", "log analysis", "logging", "tail -f",
		"metric", "grafana", "prometheus",
		// Common SRE commands
		"top ", "htop", "df ", "du ", "free ", "iostat", "netstat",
		"vm_stat", "sysctl", "lscpu", "ps aux",
	}
	for _, keyword := range sreKeywords {
		if strings.Contains(taskLower, keyword) {
			return kyoci.RoleSRE
		}
	}

	// ── 3. QA — testing/review/security ──────────────────────────────
	// Checked BEFORE developer so "write test cases" routes to QA.
	qaKeywords := []string{
		"test case", "test cases", "write test", "run test", "unit test",
		"integration test", "e2e test", "test suite", "test coverage",
		"bug", "regression", "qa ", "quality assur",
		"review", "security scan", "vulnerab", "audit",
		"assert", "expect", "mock", "stub", "fixture",
		"pytest", "jest", "mocha", "cypress", "playwright",
	}
	for _, keyword := range qaKeywords {
		if strings.Contains(taskLower, keyword) {
			return kyoci.RoleQA
		}
	}

	// ── 4. PM — planning/coordination ────────────────────────────────
	pmKeywords := []string{
		"project timeline", "project plan", "roadmap", "sprint",
		"prioritize", "schedule", "milestone", "backlog",
		"stakeholder", "resource allocat", "risk assess",
		"gantt", "agile plan", "scrum",
	}
	for _, keyword := range pmKeywords {
		if strings.Contains(taskLower, keyword) {
			return kyoci.RolePM
		}
	}

	// ── 5. Developer — general coding (DEFAULT FALLBACK) ─────────────
	// Developer is the most general role — anything code/file related that
	// didn't match a more specific role above goes here.
	return kyoci.RoleDeveloper
}
