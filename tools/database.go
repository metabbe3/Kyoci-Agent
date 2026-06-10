package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// DatabaseTool queries databases
type DatabaseTool struct{}

// NewDatabaseTool creates a new Database tool
func NewDatabaseTool() *DatabaseTool {
	return &DatabaseTool{}
}

// DBParams holds database tool parameters
type DBParams struct {
	Action           string `json:"action"`
	ConnectionString string `json:"connection_string"`
	Query            string `json:"query"`
	DBType           string `json:"db_type"`
}

func (t *DatabaseTool) Name() string {
	return "database"
}

func (t *DatabaseTool) Description() string {
	return "Query databases (PostgreSQL, MySQL, SQLite). Supports listing tables, describing table structure, and executing custom queries. Uses database CLI tools (psql, mysql, sqlite3)."
}

func (t *DatabaseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"query", "list_tables", "describe"},
				"description": "Action to perform: query (execute SQL), list_tables (list all tables), describe (describe table structure)",
			},
			"connection_string": map[string]interface{}{
				"type":        "string",
				"description": "Database connection string (for SQLite: file path; for PostgreSQL/MySQL: connection parameters)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query to execute (required when action='query')",
			},
			"db_type": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"postgres", "mysql", "sqlite"},
				"description": "Database type: postgres, mysql, or sqlite",
			},
		},
		"required": []string{"action", "db_type", "connection_string"},
	}
}

func (t *DatabaseTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params DBParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	// Validate action
	if params.Action != "query" && params.Action != "list_tables" && params.Action != "describe" {
		return "", fmt.Errorf("invalid action: %s. Must be one of: query, list_tables, describe", params.Action)
	}

	// Validate db_type
	if params.DBType != "postgres" && params.DBType != "mysql" && params.DBType != "sqlite" {
		return "", fmt.Errorf("unsupported database type: %s. Must be one of: postgres, mysql, sqlite", params.DBType)
	}

	// Validate connection_string
	if params.ConnectionString == "" {
		return "", fmt.Errorf("connection_string is required")
	}

	// Validate query for query action
	if params.Action == "query" && params.Query == "" {
		return "", fmt.Errorf("query is required when action='query'")
	}

	// Validate query for describe action
	if params.Action == "describe" && params.Query == "" {
		return "", fmt.Errorf("query (table name) is required when action='describe'")
	}

	// Execute based on db_type
	var result string
	var err error

	switch params.DBType {
	case "postgres":
		result, err = t.executePostgres(ctx, params)
	case "mysql":
		result, err = t.executeMySQL(ctx, params)
	case "sqlite":
		result, err = t.executeSQLite(ctx, params)
	}

	if err != nil {
		return result, err
	}

	return result, nil
}

func (t *DatabaseTool) executePostgres(ctx context.Context, params DBParams) (string, error) {
	var args []string
	var query string

	switch params.Action {
	case "list_tables":
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name;`
	case "describe":
		query = fmt.Sprintf(`SELECT column_name, data_type, is_nullable, column_default FROM information_schema.columns WHERE table_name = '%s' ORDER BY ordinal_position;`, params.Query)
	case "query":
		query = params.Query
	}

	// Build psql command
	args = []string{
		params.ConnectionString,
		"-c", query,
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, "psql", args...)

	// Execute and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("psql command failed: %w", err)
	}

	return string(output), nil
}

func (t *DatabaseTool) executeMySQL(ctx context.Context, params DBParams) (string, error) {
	var args []string
	var query string

	switch params.Action {
	case "list_tables":
		query = "SHOW TABLES;"
	case "describe":
		query = fmt.Sprintf("DESCRIBE `%s`;", params.Query)
	case "query":
		query = params.Query
	}

	// Handle connection string format: user:***@tcp(host:port)/dbname
	if strings.Contains(params.ConnectionString, "@tcp(") {
		args = append([]string{params.ConnectionString}, "-e", query)
	} else if strings.Contains(params.ConnectionString, ":") && !strings.HasPrefix(params.ConnectionString, "/") {
		// Assume format: user:password@host/dbname
		args = append([]string{params.ConnectionString}, "-e", query)
	} else {
		// Just add it as an argument
		args = append([]string{params.ConnectionString}, "-e", query)
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, "mysql", args...)

	// Execute and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("mysql command failed: %w", err)
	}

	return string(output), nil
}

func (t *DatabaseTool) executeSQLite(ctx context.Context, params DBParams) (string, error) {
	var query string

	switch params.Action {
	case "list_tables":
		query = "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;"
	case "describe":
		query = fmt.Sprintf("PRAGMA table_info(`%s`);", params.Query)
	case "query":
		query = params.Query
	}

	// Build sqlite3 command
	args := []string{
		params.ConnectionString,
		query,
	}

	// Create command with context
	cmd := exec.CommandContext(ctx, "sqlite3", args...)

	// Execute and capture output
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("sqlite3 command failed: %w", err)
	}

	return string(output), nil
}