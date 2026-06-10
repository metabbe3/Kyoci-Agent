package main

import (
	"context"
	"fmt"

	"github.com/nicholas/ai-agent/skill"
)

func main() {
	// Create a new skill registry
	registry := skill.NewRegistry()

	// Register built-in zero-AI skills
	if err := skill.RegisterBuiltinSkills(registry); err != nil {
		fmt.Printf("Error registering builtin skills: %v\n", err)
		return
	}

	fmt.Println("=== Zero-AI Skill Registry Demo ===")
	fmt.Println()

	// Test math skill
	testInput := "calculate 2 + 3 * 4"
	result, found, err := registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test time skill
	testInput = "what time"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test hash skill
	testInput = "sha256 hello world"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test encode skill
	testInput = "base64 encode hello"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test UUID skill
	testInput = "generate uuid"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test health skill
	testInput = "ping"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test unit convert skill
	testInput = "convert 100 celsius"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput: %s\n\n", testInput, result)
	}

	// Test JSON format skill
	testInput = "format json {\"name\":\"test\",\"value\":123}"
	result, found, err = registry.Execute(context.Background(), testInput)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else if found {
		fmt.Printf("Input: %s\nOutput:\n%s\n\n", testInput, result)
	}

	// List all registered skills
	fmt.Println("=== Registered Skills ===")
	names := registry.List()
	for _, name := range names {
		if desc, ok := registry.Describe(name); ok {
			fmt.Printf("- %s: %s\n", name, desc)
		}
	}

	// Generate a custom skill
	fmt.Println("\n=== Generating Custom Skill ===")
	if err := registry.GenerateSkill("greet", `say\s+hello\s+to\s+(\w+)`, "Hello, {0}! Welcome!"); err != nil {
		fmt.Printf("Error generating skill: %v\n", err)
	} else {
		fmt.Println("Custom skill 'greet' generated successfully")
		
		testInput = "say hello to Alice"
		result, found, err = registry.Execute(context.Background(), testInput)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else if found {
			fmt.Printf("Input: %s\nOutput: %s\n", testInput, result)
		}
	}
}