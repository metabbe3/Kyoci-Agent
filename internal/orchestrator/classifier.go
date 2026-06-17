package orchestrator

import (
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// ClassifyRole auto-detects which role should handle a task based on heuristic
// keyword scoring plus strong domain anchors. Pure Go — no LLM call.
//
// Routing philosophy:
//
//  1. Each specialist role has a weighted keyword list. A weak substring
//     match scores 1; a strong domain anchor (file extension, framework
//     name with surrounding whitespace, etc.) scores 3.
//  2. A specialist wins only if its score >= 2 — a single accidental
//     substring match (e.g. "ui " inside "quit") is NOT enough.
//  3. If no specialist reaches 2, the task routes to Generalist. This is
//     the critical fix: previously Developer was the fallback, but
//     Developer's prompt forbids prose responses — so research /
//     explanation tasks were mishandled.
//  4. Specialist priority on ties: Frontend > SRE > QA > Developer > PM.
//     Frontend wins over Developer because "react" / "css" / "tsx" are
//     unambiguous frontend signals that would otherwise be lost in
//     Developer's catch-all net.
//
// Why no LLM routing: a small model making a bad route decision costs a
// full pipeline run (planner + workers + synthesizer). A pure-Go heuristic
// is faster, deterministic, and debuggable.
func ClassifyRole(task string) kyoci.RoleType {
	taskLower := strings.ToLower(task)
	scores := map[kyoci.RoleType]int{}

	// ── Score helpers ─────────────────────────────────────────────────
	add := func(rt kyoci.RoleType, keywords ...string) {
		for _, kw := range keywords {
			if strings.Contains(taskLower, kw) {
				scores[rt]++
			}
		}
	}
	strong := func(rt kyoci.RoleType, anchors ...string) {
		for _, a := range anchors {
			if strings.Contains(taskLower, a) {
				scores[rt] += 3
			}
		}
	}

	// ── Frontend ──────────────────────────────────────────────────────
	strong(kyoci.RoleFrontend,
		".html", ".css", ".scss", ".tsx", ".jsx",
		" react ", " reactjs", " next.js", " nextjs", " vue ", " svelte ",
		" astro ", "tailwind", "css grid", "flexbox",
	)
	add(kyoci.RoleFrontend,
		"html", "css", "frontend", "ui ", "ux ",
		"component", "button", "navbar", "sidebar", "footer", "landing page",
		"responsive", "media query",
		"dom", "accessibility", "aria", "typescript",
		"web page", "webpage", "website design",
	)

	// ── SRE ───────────────────────────────────────────────────────────
	strong(kyoci.RoleSRE,
		"kubernetes", " k8s ", "docker", "nginx", "grafana", "prometheus",
		"deploy ", "deployment", "auto-scal", "autoscal",
		"health check", "health-check",
	)
	add(kyoci.RoleSRE,
		"disk space", "disk usage", "cpu ", "memory usage", "ram ",
		"system performance", "machine performance", "server load",
		"uptime", "health status",
		"container", "load balanc", "scaling",
		"monitor", "alert", "incident", "outage",
		"infra", "ops ", "production server", "staging",
		"port", "firewall", "dns", "ssl", "certificate",
		"network", "connection", "ping ", "latency",
		"log file", "log analysis", "logging", "tail -f",
		"metric",
		"top ", "htop", "df ", "du ", "free ", "iostat", "netstat",
		"vm_stat", "sysctl", "lscpu", "ps aux",
	)

	// ── QA ────────────────────────────────────────────────────────────
	strong(kyoci.RoleQA,
		"_test.go", "_test.py", "_test.js", "_test.ts",
		"pytest", " jest ", " mocha ", "cypress", "playwright",
		"security scan", "vulnerab",
	)
	add(kyoci.RoleQA,
		"test case", "test cases", "write test", "run test", "unit test",
		"integration test", "e2e test", "test suite", "test coverage",
		"bug", "regression", "qa ", "quality assur",
		"review", "audit",
		"assert", "expect", "mock", "stub", "fixture",
	)

	// ── Developer ─────────────────────────────────────────────────────
	// NOTE: language names ("rust", "python", "go") are deliberately NOT
	// strong anchors — a user asking "explain the rust async ecosystem"
	// should route to Generalist, not Developer. Build-tool commands and
	// file extensions are unambiguous; language names are not.
	strong(kyoci.RoleDeveloper,
		".go ", ".go:", ".py ", ".py:", ".rs ", ".java ",
		" go build", " go run", " go test", " cargo ", " pip install",
		" npm install",
	)
	add(kyoci.RoleDeveloper,
		"function", "method", "class", "struct", "interface",
		"api", "endpoint", "refactor",
		"algorithm", "data structure",
		"debug", "stack trace", "exception",
		"compile", "build error",
	)

	// ── PM ────────────────────────────────────────────────────────────
	strong(kyoci.RolePM,
		"roadmap", "gantt", "scrum", "agile plan",
		"project plan", "project timeline",
	)
	add(kyoci.RolePM,
		"sprint", "prioritize", "schedule", "milestone", "backlog",
		"stakeholder", "resource allocat", "risk assess",
	)

	// ── Pick the winner ───────────────────────────────────────────────
	// Specialists need score >= 2 to win — single substring matches are
	// not enough. If nobody clears the bar, route to Generalist.
	const minScore = 2

	type candidate struct {
		role    kyoci.RoleType
		priority int // lower = higher priority (wins ties)
	}
	tiebreaker := []candidate{
		{kyoci.RoleFrontend, 1}, // most specific
		{kyoci.RoleSRE, 2},
		{kyoci.RoleQA, 3},
		{kyoci.RoleDeveloper, 4},
		{kyoci.RolePM, 5},
	}

	var best kyoci.RoleType
	var bestScore int
	for _, c := range tiebreaker {
		s := scores[c.role]
		if s > bestScore {
			best = c.role
			bestScore = s
		}
	}

	if bestScore >= minScore {
		return best
	}
	return kyoci.RoleGeneralist
}
