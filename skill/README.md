# Skill Package

Zero-AI skill registry with built-in handlers for basic tasks. This package provides a pattern-matching system to execute simple, deterministic operations without requiring AI models.

## Features

- **Pattern-based routing**: Match user input using regular expressions
- **Zero-AI handlers**: Pure Go implementations for common tasks
- **Extensible**: Easy to add new skills or generate them from templates
- **Thread-safe**: Concurrent access to the registry is protected by RWMutex

## Core Types

### SkillHandler
```go
type SkillHandler func(ctx context.Context, input string) (string, error)
```

A function that processes input and returns a result string or error.

### Skill
```go
type Skill struct {
    Name        string
    Pattern     *regexp.Regexp
    Handler     SkillHandler
    Description string
}
```

Represents a zero-AI capability with a pattern matcher.

### Registry
```go
type Registry struct {
    skills map[string]*Skill
    mu     sync.RWMutex
}
```

Manages zero-AI skill handlers.

## Usage

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "github.com/nicholas/ai-agent/skill"
)

func main() {
    // Create registry and register built-in skills
    registry := skill.NewRegistry()
    skill.RegisterBuiltinSkills(registry)
    
    // Execute a skill
    result, found, err := registry.Execute(context.Background(), "calculate 2 + 3")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
    } else if found {
        fmt.Printf("Result: %s\n", result) // Output: Result: 2 + 3 = 5
    }
}
```

### Registering Custom Skills

```go
handler := func(ctx context.Context, input string) (string, error) {
    return "Custom response", nil
}

err := registry.Register("my_skill", `(?i)my pattern`, handler, "My custom skill")
```

### Generating Skills from Templates

```go
// Create a skill with template-based capture group substitution
err := registry.GenerateSkill(
    "greet",
    `say\s+hello\s+to\s+(\w+)`,
    "Hello, {0}! Welcome!",
)

// Input: "say hello to Alice"
// Output: "Hello, Alice! Welcome!"
```

## Built-in Skills

### Math
**Pattern**: `(?i)(hitung|calculate|compute|berapa)\s+([\d+\-*/(). ]+)`

Evaluates simple arithmetic expressions (supports +, -, *, /, and parentheses).

```go
// Input: "calculate 2 + 3 * 4"
// Output: "Result: 2 + 3 * 4 = 14"
```

### Time
**Pattern**: `(?i)(jam berapa|what time|current time|tanggal|what date)`

Returns current time or date.

```go
// Input: "what time"
// Output: "Current time: 14:30:45"
```

### Hash
**Pattern**: `(?i)(hash|sha256|md5)\s+(.+)`

Computes SHA256 or MD5 hash of input.

```go
// Input: "sha256 hello world"
// Output: "SHA256(hello world) = a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"
```

### Encode/Decode
**Pattern**: `(?i)(base64 (encode|decode))\s+(.+)`

Base64 encode or decode input.

```go
// Input: "base64 encode hello"
// Output: "Base64(hello) = aGVsbG8="
```

### UUID
**Pattern**: `(?i)(generate|buat|create)\s+(uuid|id)`

Generates a random UUID (version 4).

```go
// Input: "generate uuid"
// Output: "Generated UUID: 550e8400-e29b-41d4-a716-446655440000"
```

### JSON Format
**Pattern**: `(?i)(format|pretty)\s+json\s+(.+)`

Pretty-prints JSON input.

```go
// Input: "format json {\"name\":\"test\",\"value\":123}"
// Output:
// {
//   "name": "test",
//   "value": 123
// }
```

### Health
**Pattern**: `(?i)(ping|health|alive|status)`

Simple health check.

```go
// Input: "ping"
// Output: "OK - System is healthy and responsive"
```

### Unit Convert
**Pattern**: `(?i)(convert|konversi)\s+(\d+)\s+(celsius|fahrenheit|km|mi|kg|lb)`

Converts between common units.

```go
// Input: "convert 100 celsius"
// Output: "100.00°C = 212.00°F"
```

## API Reference

### Registry Methods

#### `NewRegistry() *Registry`
Creates a new skill registry.

#### `Register(name, pattern string, handler SkillHandler, desc string) error`
Adds a new skill to the registry. Returns an error if the pattern is invalid.

#### `Match(input string) (*Skill, bool)`
Checks if input matches any registered skill pattern.

#### `Execute(ctx context.Context, input string) (string, bool, error)`
Tries to match and execute a skill for the given input.

#### `List() []string`
Returns all registered skill names.

#### `Get(name string) (*Skill, bool)`
Retrieves a skill by name.

#### `Unregister(name string)`
Removes a skill from the registry.

#### `Count() int`
Returns the number of registered skills.

#### `Describe(name string) (string, bool)`
Returns the description of a skill by name.

#### `GenerateSkill(name, pattern, template string) error`
Creates a new skill from a pattern and template. Template uses {0}, {1}, etc. for regex capture groups.

#### `GenerateToFile(name, pattern, template string) error`
Generates a new skill and saves its code to `skill/auto_<name>.go`.

## Running the Demo

```bash
cd examples
go run skill_demo.go
```

## License

MIT License