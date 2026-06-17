package orchestrator

import (
	"testing"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// TestClassifyRole_SpecialistRouting locks in the contract that clear specialist
// signals route to the matching specialist. Strong anchors (file extensions,
// framework names) and any two weak hits should both be enough.
func TestClassifyRole_SpecialistRouting(t *testing.T) {
	cases := []struct {
		name string
		task string
		want kyoci.RoleType
	}{
		// Frontend — strong anchors
		{"react component", "build a react dashboard with charts", kyoci.RoleFrontend},
		{"css file", "fix the broken css in styles.css", kyoci.RoleFrontend},
		{"tsx file", "refactor this .tsx component", kyoci.RoleFrontend},
		{"tailwind", "switch the page to tailwind utilities", kyoci.RoleFrontend},
		// Frontend — two weak hits
		{"ui component", "build a button component for the ui", kyoci.RoleFrontend},

		// SRE — strong anchors
		{"k8s deploy", "deploy to kubernetes with health checks", kyoci.RoleSRE},
		{"docker container", "debug the docker container that won't start", kyoci.RoleSRE},
		{"nginx config", "fix the nginx config for ssl termination", kyoci.RoleSRE},
		// SRE — two weak hits
		{"disk + memory", "check disk space and memory usage on the server", kyoci.RoleSRE},

		// QA — strong anchors
		{"pytest file", "add a pytest file for the auth module", kyoci.RoleQA},
		{"go test file", "write tests in parser_test.go for the new branch", kyoci.RoleQA},
		// QA — two weak hits
		{"write test cases", "write test cases for the user service", kyoci.RoleQA},

		// Developer — strong anchors
		{"go build", "run go build and fix any errors", kyoci.RoleDeveloper},
		{"python file", "refactor this .py file to use async", kyoci.RoleDeveloper},
		// Developer — two weak hits
		{"refactor function", "refactor the function and add error handling", kyoci.RoleDeveloper},

		// PM — strong anchors
		{"project plan", "draft a project plan for the Q3 roadmap", kyoci.RolePM},
		{"scrum sprint", "plan the next scrum sprint", kyoci.RolePM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyRole(c.task)
			if got != c.want {
				t.Errorf("ClassifyRole(%q) = %s, want %s", c.task, got, c.want)
			}
		})
	}
}

// TestClassifyRole_GeneralistFallback verifies the core fix: tasks with no
// clear specialist signal route to Generalist, NOT Developer. This is what
// stops research/explanation questions from being mishandled by Developer's
// "no prose in response" rule.
func TestClassifyRole_GeneralistFallback(t *testing.T) {
	cases := []struct {
		name string
		task string
	}{
		{"explanation", "explain how DNS resolution works"},
		{"comparison", "compare postgres and mysql for a small project"},
		{"research", "what's the latest on the rust async ecosystem"},
		{"summarize", "summarize this article in three bullets"},
		{"math from prose", "what's 23 percent of 4500"},
		{"ambiguous one-word", "help"},
		{"greeting", "hi, what can you do"},
		// Single accidental substring match must NOT win.
		// "quit" contains "ui" — that alone must not route to Frontend.
		{"accidental substring", "I want to quit my subscription"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ClassifyRole(c.task)
			if got != kyoci.RoleGeneralist {
				t.Errorf("ClassifyRole(%q) = %s, want %s (single accidental match must not win)",
					c.task, got, kyoci.RoleGeneralist)
			}
		})
	}
}

// TestClassifyRole_QAWinsOverDeveloper ensures the priority tiebreaker still
// routes test-writing tasks to QA rather than letting Developer's broad
// "function / method / api" keyword net swallow them.
func TestClassifyRole_QAWinsOverDeveloper(t *testing.T) {
	// "write test cases for the parser function" hits BOTH Developer (function)
	// and QA (test cases, write test). QA must win — testing is the intent.
	task := "write test cases for the parser function"
	got := ClassifyRole(task)
	if got != kyoci.RoleQA {
		t.Errorf("ClassifyRole(%q) = %s, want qa (testing intent must outrank generic code intent)",
			task, got)
	}
}
