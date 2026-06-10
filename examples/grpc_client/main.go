package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nicholas/ai-agent/grpc"
)

func main() {
	client, err := grpc.NewClient("localhost:50051")
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simple chat
	fmt.Println("--- Chat Example ---")
	resp, err := client.Chat(context.Background(), "Hello! Who are you?")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Response: %s\n", resp.Message)
	fmt.Printf("Model: %s\n", resp.ModelUsed)

	// Streaming chat
	fmt.Println("\n--- Streaming Chat Example ---")
	ch, err := client.ChatStream(context.Background(), "Count to 5")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print("Stream: ")
	for chunk := range ch {
		fmt.Print(chunk)
	}
	fmt.Println()

	// Execute tool (example with calculator)
	fmt.Println("\n--- Execute Tool Example ---")
	toolResp, err := client.ExecuteTool(context.Background(), "calculator", `{"expression": "2+2"}`)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Tool result: %s (error: %v)\n", toolResp.Result, toolResp.Error)

	// Get status
	fmt.Println("\n--- Status Example ---")
	status, err := client.GetStatus(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Uptime: %ds\n", status.UptimeSeconds)
	fmt.Printf("Tools: %d\n", status.ToolsCount)
	fmt.Printf("Memory: total=%d heap=%d sys=%d gc=%d\n",
		status.Memory.TotalAlloc,
		status.Memory.HeapAlloc,
		status.Memory.Sys,
		status.Memory.GcCount)
	fmt.Printf("Providers:\n")
	for _, p := range status.Providers {
		fmt.Printf("  - %s: %s (available: %v)\n", p.Name, p.Model, p.Available)
	}
}