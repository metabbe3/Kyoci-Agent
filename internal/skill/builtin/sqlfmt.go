package builtin

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// SQLFmtSkill is a basic SQL formatter that uppercases keywords and puts each
// major clause on its own line.
type SQLFmtSkill struct {
	*kyoci.BaseSkill
}

// NewSQLFmtSkill creates a new sql formatter skill.
func NewSQLFmtSkill() *SQLFmtSkill {
	return &SQLFmtSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"sqlfmt",
			"Basic SQL formatter — uppercases keywords, indents",
			[]string{"sql", "format sql", "sql format"},
		),
	}
}

// Match checks if the query references SQL.
func (s *SQLFmtSkill) Match(query string) bool {
	return strings.Contains(strings.ToLower(query), "sql")
}

// Execute formats the SQL text found in the query.
func (s *SQLFmtSkill) Execute(ctx context.Context, query string) (string, error) {
	queryLower := strings.ToLower(query)
	sql := strings.TrimSpace(query)

	// Strip a leading verb so the formatter doesn't include it as SQL.
	sql = stripSQLVerb(sql, queryLower)

	if sql == "" {
		return "", fmt.Errorf("no SQL content found in query")
	}

	sql = strings.TrimSpace(sql)
	// Unquote in case the user wrapped the SQL in quotes/backticks.
	if len(sql) >= 2 {
		if (sql[0] == '"' && sql[len(sql)-1] == '"') || (sql[0] == '`' && sql[len(sql)-1] == '`') {
			sql = sql[1 : len(sql)-1]
		}
	}

	formatted := formatSQL(sql)
	return fmt.Sprintf("Formatted SQL:\n\n%s", formatted), nil
}

func stripSQLVerb(s, lowered string) string {
	for _, w := range []string{"format sql", "sql format", "format", "sql", "pretty"} {
		prefix := w + " "
		if strings.HasPrefix(lowered, prefix) {
			return s[len(prefix):]
		}
	}
	return s
}

// majorKeywords are SQL keywords that should each begin a new line.
var majorKeywords = []string{
	"SELECT", "FROM", "WHERE", "JOIN", "LEFT JOIN", "RIGHT JOIN", "INNER JOIN",
	"OUTER JOIN", "FULL JOIN", "ON", "GROUP BY", "ORDER BY", "HAVING", "LIMIT",
	"OFFSET", "INSERT INTO", "INSERT", "UPDATE", "DELETE FROM", "DELETE",
	"VALUES", "SET", "AND", "OR", "NOT NULL", "UNION", "UNION ALL",
}

// allKeywords are uppercased everywhere they appear as standalone tokens.
var allKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "JOIN": true, "ON": true,
	"GROUP": true, "BY": true, "ORDER": true, "HAVING": true, "LIMIT": true,
	"OFFSET": true, "INSERT": true, "INTO": true, "UPDATE": true, "DELETE": true,
	"VALUES": true, "SET": true, "AND": true, "OR": true, "NOT": true, "NULL": true,
	"INNER": true, "LEFT": true, "RIGHT": true, "FULL": true, "OUTER": true,
	"UNION": true, "ALL": true, "AS": true, "DISTINCT": true, "IN": true,
	"IS": true, "LIKE": true, "BETWEEN": true, "ASC": true, "DESC": true,
	"CREATE": true, "TABLE": true, "DROP": true, "ALTER": true, "ADD": true,
	"PRIMARY": true, "KEY": true, "FOREIGN": true, "REFERENCES": true,
	"DEFAULT": true, "UNIQUE": true, "INDEX": true,
}

// tokenize splits SQL into whitespace-delimited tokens but keeps quoted
// string literals and parenthesized groups intact enough for keyword casing.
func tokenize(sql string) []string {
	var tokens []string
	var cur strings.Builder
	inSingle := false
	inDouble := false
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			cur.WriteByte(c)
		case c == '"' && !inSingle:
			inDouble = !inDouble
			cur.WriteByte(c)
		case (c == ' ' || c == '\t' || c == '\n' || c == '\r') && !inSingle && !inDouble:
			flush()
		case (c == ',' || c == '(' || c == ')' || c == ';') && !inSingle && !inDouble:
			flush()
			tokens = append(tokens, string(c))
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return tokens
}

func formatSQL(sql string) string {
	// Collapse runs of whitespace into single spaces for uniform tokenizing.
	sql = strings.Join(strings.Fields(sql), " ")

	// Uppercase standalone keywords (preserving identifiers, strings, etc.).
	tokens := tokenize(sql)
	for i, tok := range tokens {
		upper := strings.ToUpper(tok)
		if allKeywords[upper] {
			tokens[i] = upper
		}
	}
	sql = strings.Join(tokens, " ")

	// Insert newlines before each major keyword.
	for _, kw := range majorKeywords {
		// Match " KW" not at the very start of the string.
		target := " " + kw + " "
		repl := "\n" + kw + " "
		// Only replace whole-word occurrences (case-sensitive because we already uppercased).
		sql = replaceKeyword(sql, kw, repl)
		_ = target
	}

	// Normalize multiple newlines and trim each line.
	lines := strings.Split(sql, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	out := strings.Join(lines, "\n")
	// Clean up any residual double-blank lines.
	for strings.Contains(out, "\n\n") {
		out = strings.ReplaceAll(out, "\n\n", "\n")
	}
	return out
}

// replaceKeyword inserts a newline before standalone occurrences of kw (uppercase).
func replaceKeyword(s, kw, repl string) string {
	// Build a token-aware replacement: scan for " KW " boundaries.
	needle := " " + kw + " "
	idx := 0
	var b strings.Builder
	b.WriteString(s)
	// Work on a copy because we mutate via builder.
	src := b.String()
	b.Reset()
	i := 0
	for {
		j := indexFoldFrom(src, needle, i, kw)
		if j < 0 {
			b.WriteString(src[i:])
			break
		}
		// Write up to j, then replacement.
		b.WriteString(src[i:j])
		// replacement = "\nKW "
		b.WriteString(repl)
		i = j + len(needle)
		idx++
		_ = idx
	}
	return b.String()
}

// indexFoldFrom searches for needle starting at position i, matching kw case-insensitively.
// Since sql has already been uppercased at the keyword level, a direct substring search suffices.
func indexFoldFrom(s, needle string, from int, kw string) int {
	if from > len(s) {
		return -1
	}
	return strings.Index(s[from:], needle) + from - 1
}
