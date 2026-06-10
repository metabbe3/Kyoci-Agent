package classifier

import (
	"strings"
)

// PatternGroup maps patterns to categories for each level
type PatternGroup struct {
	Patterns []string
	Category string
}

// Level 1 (Trivial) patterns - greetings, simple questions, basic math
var Level1Patterns = []PatternGroup{
	{
		Category: "greeting",
		Patterns: []string{
			"hello", "hi", "hey", "greetings", "good morning", "good afternoon", "good evening",
			"how are you", "how's it going", "what's up", "hi there", "hey there",
		},
	},
	{
		Category: "farewell",
		Patterns: []string{
			"goodbye", "bye", "see you", "see ya", "farewell", "take care", "good night",
		},
	},
	{
		Category: "acknowledgment",
		Patterns: []string{
			"thank", "thanks", "ok", "okay", "alright", "got it", "understood", "sure", "yes", "no",
		},
	},
	{
		Category: "basic_math",
		Patterns: []string{
			"what is", "calculate", "add", "subtract", "multiply", "divide", "plus", "minus",
			"times", "divided by", "equals", "is", "what's", "what does",
		},
	},
	{
		Category: "time_date",
		Patterns: []string{
			"what time", "what date", "what day", "current time", "current date", "today is",
		},
	},
}

// Level 2 (Simple) patterns - formatting, parsing, single-step lookups
var Level2Patterns = []PatternGroup{
	{
		Category: "formatting",
		Patterns: []string{
			"format", "capitalize", "uppercase", "lowercase", "reverse", "trim",
			"convert", "encode", "decode", "base64", "json", "xml", "csv",
		},
	},
	{
		Category: "parsing",
		Patterns: []string{
			"parse", "extract", "find", "search", "match", "replace", "split", "join",
		},
	},
	{
		Category: "lookup",
		Patterns: []string{
			"what is the definition", "define", "meaning of", "tell me about",
			"list", "show me", "get", "retrieve", "fetch",
		},
	},
	{
		Category: "command",
		Patterns: []string{
			"list files", "ls", "pwd", "cd", "mkdir", "touch", "echo",
			"show directory", "current path", "create file", "create directory",
		},
	},
	{
		Category: "status",
		Patterns: []string{
			"status", "version", "info", "help", "what can you do", "capabilities",
		},
	},
}

// Level 3 (Moderate) patterns - multi-step reasoning, basic coding, summarization
var Level3Patterns = []PatternGroup{
	{
		Category: "summarization",
		Patterns: []string{
			"summarize", "summary", "brief", "condense", "abstract",
			"key points", "main idea", "overview", "recap",
		},
	},
	{
		Category: "basic_coding",
		Patterns: []string{
			"write a function", "create a function", "implement", "code",
			"write a script", "create a script", "small script",
		},
	},
	{
		Category: "explanation",
		Patterns: []string{
			"explain how", "why does", "how does", "describe the process",
			"step by step", "walk through", "breakdown",
		},
	},
	{
		Category: "comparison",
		Patterns: []string{
			"compare", "difference between", "vs", "versus", "which is better",
			"pros and cons", "advantages", "disadvantages",
		},
	},
	{
		Category: "translation",
		Patterns: []string{
			"translate to", "translate", "convert language", "in spanish",
			"in french", "in german", "in japanese",
		},
	},
}

// Level 4 (Complex) patterns - architecture decisions, debugging, analysis
var Level4Patterns = []PatternGroup{
	{
		Category: "debugging",
		Patterns: []string{
			"debug", "fix bug", "error", "exception", "not working",
			"troubleshoot", "diagnose", "issue", "problem with",
		},
	},
	{
		Category: "architecture",
		Patterns: []string{
			"architecture", "design pattern", "best practice", "optimize",
			"refactor", "improve", "efficient", "scalable",
		},
	},
	{
		Category: "analysis",
		Patterns: []string{
			"analyze", "evaluate", "assess", "review", "audit",
			"inspect", "examine", "investigate",
		},
	},
	{
		Category: "integration",
		Patterns: []string{
			"integrate", "connect", "setup", "configure", "deploy",
			"environment", "installation", "dependencies",
		},
	},
	{
		Category: "advanced_coding",
		Patterns: []string{
			"implement algorithm", "data structure", "api design",
			"database schema", "microservice", "design system",
		},
	},
}

// Level 5 (Critical) patterns - multi-system integration, creative work, research
var Level5Patterns = []PatternGroup{
	{
		Category: "creative",
		Patterns: []string{
			"write a story", "write a poem", "creative writing", "compose",
			"brainstorm", "imagine", "invent", "novel idea",
		},
	},
	{
		Category: "research",
		Patterns: []string{
			"research", "study", "investigate deep", "comprehensive",
			"literature review", "survey", "academic",
		},
	},
	{
		Category: "multi_system",
		Patterns: []string{
			"system integration", "end-to-end", "full stack", "platform",
			"enterprise", "distributed system", "complex workflow",
		},
	},
	{
		Category: "strategic",
		Patterns: []string{
			"strategy", "roadmap", "plan", "vision", "long-term",
			"business case", "proposal", "recommendation",
		},
	},
	{
		Category: "advanced_research",
		Patterns: []string{
			"deep learning", "machine learning", "ai model", "algorithm",
			"optimization", "theoretical", "cutting edge",
		},
	},
}

// MatchesPatterns checks if input matches any patterns in the group
func MatchesPatterns(input string, patternGroups []PatternGroup) ([]string, string) {
	inputLower := strings.ToLower(input)

	// Pre-allocate with estimated capacity
	totalPatterns := 0
	for _, group := range patternGroups {
		totalPatterns += len(group.Patterns)
	}
	matchedPatterns := make([]string, 0, totalPatterns/2) // Estimate half may match
	var category string

	for _, group := range patternGroups {
		for _, pattern := range group.Patterns {
			if strings.Contains(inputLower, strings.ToLower(pattern)) {
				matchedPatterns = append(matchedPatterns, pattern)
				category = group.Category
			}
		}
	}

	return matchedPatterns, category
}