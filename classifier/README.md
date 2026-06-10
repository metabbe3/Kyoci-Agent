# Classifier Package

A rule-based complexity classifier for AI agent task routing. This package analyzes input and classifies it into 5 complexity levels to determine the appropriate processing approach (code-only vs AI) and model selection.

## Features

- **5-Level Complexity Classification**: Trivial, Simple, Moderate, Complex, Critical
- **Pattern-Based Detection**: Keyword patterns for each complexity level
- **Skill Matching**: Exact and near-exact skill invocation detection
- **Heuristic Analysis**: Input length, code detection, tool requirements, question types
- **Thread-Safe Skill Registry**: Register and match available skills
- **Token Budget Recommendations**: Suggested token limits per level
- **Model Recommendations**: Local/small, mid-tier, or best AI models

## Complexity Levels

| Level | Name           | Token Budget | AI Required | Model         | Description                          |
|-------|----------------|--------------|-------------|---------------|--------------------------------------|
| 1     | Trivial        | 0            | No          | -             | Greetings, simple questions, basic math |
| 2     | Simple         | 0            | No          | -             | Formatting, parsing, single-step lookups |
| 3     | Moderate       | 1024         | Yes         | local/small   | Multi-step reasoning, basic coding, summarization |
| 4     | Complex        | 4096         | Yes         | mid-tier      | Architecture decisions, debugging, analysis |
| 5     | Critical       | 8192         | Yes         | best          | Multi-system integration, creative work, research |

## Usage

```go
package main

import (
    "fmt"
    "github.com/nicholas/ai-agent/classifier"
)

func main() {
    // Initialize default skills
    classifier.InitializeDefaultSkills()

    // Classify input
    result := classifier.Classify("hello")
    fmt.Printf("Level: %d, Category: %s, NeedsAI: %v\n",
        result.Level, result.Category, result.NeedsAI)

    // Check AI requirement
    if result.IsAIRequired() {
        fmt.Printf("Recommended model: %s, Token budget: %d\n",
            result.RecommendedModel(), result.TokenBudget)
    }

    // Get matched patterns and reasoning
    fmt.Printf("Matched patterns: %v\n", result.MatchedPatterns)
    fmt.Printf("Reasoning: %v\n", result.Reasoning)
}
```

## Skill Registration

```go
// Register a new skill
classifier.RegisterSkill(
    "my_skill",                          // name
    []string{"alias1", "alias2"},        // aliases
    "Description of my skill",          // description
    2,                                   // level (1-5)
)

// Match skills from input
skill := classifier.MatchSkill("use my_skill to process data")
if skill != "" {
    fmt.Printf("Matched skill: %s\n", skill)
}

// List all skills
skills := classifier.ListSkills()
fmt.Printf("Available skills: %v\n", skills)
```

## Pattern Examples

### Level 1 (Trivial)
- Greetings: "hello", "hi", "good morning"
- Acknowledgments: "thank", "ok", "understood"
- Basic math: "what is 2+2", "calculate 5*3"
- Time/date: "what time is it"

### Level 2 (Simple)
- Formatting: "format as JSON", "uppercase this"
- Parsing: "parse this data", "extract email addresses"
- Lookups: "define algorithm", "list files"
- Commands: "create file", "show directory"

### Level 3 (Moderate)
- Summarization: "summarize this text"
- Explanation: "explain how neural networks work"
- Comparison: "compare Python and JavaScript"
- Translation: "translate to Spanish"

### Level 4 (Complex)
- Debugging: "debug this code"
- Architecture: "design microservice architecture"
- Analysis: "analyze this dataset"
- Integration: "integrate API with database"

### Level 5 (Critical)
- Creative: "write a science fiction story"
- Research: "conduct deep research on quantum computing"
- Multi-system: "design end-to-end enterprise platform"
- Strategic: "create long-term AI integration roadmap"

## API Reference

### Main Function

- `Classify(input string) *ClassificationResult` - Analyzes input and returns classification

### ClassificationResult Methods

- `IsAIRequired() bool` - Returns true if AI processing is needed
- `NeedsAICheck() bool` - Alias for IsAIRequired
- `RecommendedModel() string` - Returns recommended model ("local/small", "mid-tier", "best")
- `GetTokenBudget() int` - Returns token budget (0, 1024, 4096, 8192)
- `Budget() int` - Alias for GetTokenBudget

### Skill Management

- `RegisterSkill(name, aliases, description string, level int)` - Register a skill
- `UnregisterSkill(name string)` - Remove a skill
- `MatchSkill(input string) string` - Find skill match in input
- `GetSkill(name string) (SkillMetadata, bool)` - Get skill metadata
- `ListSkills() []string` - List all registered skills
- `InitializeDefaultSkills()` - Initialize common skills

## Testing

Run tests with:
```bash
go test ./classifier -v
```

Run tests with coverage:
```bash
go test ./classifier -cover
```

## Example Output

```
Input: "hello"
Level: 1
Category: greeting
NeedsAI: false
TokenBudget: 0
RecommendedModel: 
MatchedPatterns: [hello]
Reasoning: [Level 1 patterns matched]

Input: "explain how microservices work"
Level: 3
Category: explanation
NeedsAI: true
TokenBudget: 1024
RecommendedModel: local/small
MatchedPatterns: [explain how]
Reasoning: [Level 3 patterns matched]

Input: "design a scalable enterprise architecture"
Level: 4
Category: architecture
NeedsAI: true
TokenBudget: 4096
RecommendedModel: mid-tier
MatchedPatterns: [architecture scalable]
Reasoning: [Level 4 patterns matched]
```

## Design Decisions

1. **Rule-Based Approach**: No AI used for classification itself - deterministic and fast
2. **Pattern Matching**: Keyword-based pattern detection for predictable results
3. **Heuristic Fallback**: When patterns don't match, heuristics classify based on input characteristics
4. **Skill Priority**: Skill matches take precedence over pattern matches
5. **Thread Safety**: Skill registry uses RWMutex for concurrent access
6. **Extensibility**: Easy to add new patterns and skills

## License

Part of the Kyoci Agent project.