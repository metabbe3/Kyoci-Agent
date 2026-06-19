package builtin

import (
	"context"
	"fmt"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// DiffSkill performs a line-by-line diff between two text inputs separated by
// ' vs ', ' --- ', or ' // '.
type DiffSkill struct {
	*kyoci.BaseSkill
}

// NewDiffSkill creates a new diff skill.
func NewDiffSkill() *DiffSkill {
	return &DiffSkill{
		BaseSkill: kyoci.NewBaseSkill(
			"diff",
			"Line-by-line diff between two text inputs (separated by ' vs ' or ' --- ' or ' // ')",
			[]string{"diff", "compare", "difference"},
		),
	}
}

// Match checks if the query references diffing AND carries one of the input
// separators Execute requires (' vs ', ' --- ', ' // '). Requiring the
// separator prevents the skill from hijacking generic "compare X and Y"
// questions (which have no separator and would make Execute error out).
func (s *DiffSkill) Match(query string) bool {
	queryLower := strings.ToLower(query)
	if !strings.Contains(queryLower, "diff") && !strings.Contains(queryLower, "compare") {
		return false
	}
	return strings.Contains(queryLower, " vs ") ||
		strings.Contains(queryLower, " --- ") ||
		strings.Contains(queryLower, " // ")
}

// Execute splits the query on a separator and produces an LCS-based diff.
func (s *DiffSkill) Execute(ctx context.Context, query string) (string, error) {
	query = strings.TrimSpace(query)

	left, right, sep, ok := splitInputs(query)
	if !ok {
		return "", fmt.Errorf("could not find separator (' vs ', ' --- ', or ' // ') in query")
	}

	leftLines := splitLines(left)
	rightLines := splitLines(right)

	ops := diffLCS(leftLines, rightLines)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Diff (separator: %q)\n\n", sep)

	changed := false
	output := make([]string, 0, len(ops))
	for _, op := range ops {
		switch op.kind {
		case opEqual:
			output = append(output, "  "+op.line)
		case opAdd:
			output = append(output, "+ "+op.line)
			changed = true
		case opDel:
			output = append(output, "- "+op.line)
			changed = true
		}
	}

	if !changed {
		sb.WriteString("(inputs are identical)\n")
		return sb.String(), nil
	}

	// Emit only changed lines plus 1 line of surrounding context.
	emitted := make([]string, 0, len(output))
	show := make([]bool, len(output))
	for i, op := range ops {
		if op.kind != opEqual {
			show[i] = true
			if i > 0 {
				show[i-1] = true
			}
			if i < len(ops)-1 {
				show[i+1] = true
			}
		}
	}
	for i, line := range output {
		if show[i] {
			emitted = append(emitted, line)
		} else if len(emitted) > 0 && emitted[len(emitted)-1] != "..." {
			emitted = append(emitted, "...")
		}
	}

	for _, line := range emitted {
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n"), nil
}

// splitInputs separates the query into left and right on the first supported separator.
func splitInputs(query string) (left, right, sep string, ok bool) {
	separators := []string{" vs ", " --- ", " // "}
	for _, sep = range separators {
		if idx := strings.Index(query, sep); idx >= 0 {
			left = strings.TrimSpace(query[:idx])
			right = strings.TrimSpace(query[idx+len(sep):])
			if left != "" && right != "" {
				return left, right, sep, true
			}
		}
	}
	return "", "", "", false
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, "\r")
	}
	return lines
}

const (
	opEqual = iota
	opAdd
	opDel
)

type diffOp struct {
	kind int
	line string
}

// diffLCS computes a line-level diff between a and b using a classic LCS table.
func diffLCS(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// dp[i][j] = length of LCS of a[i:] and b[j:]
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		if a[i] == b[j] {
			ops = append(ops, diffOp{kind: opEqual, line: a[i]})
			i++
			j++
		} else if dp[i+1][j] >= dp[i][j+1] {
			ops = append(ops, diffOp{kind: opDel, line: a[i]})
			i++
		} else {
			ops = append(ops, diffOp{kind: opAdd, line: b[j]})
			j++
		}
	}
	for i < n {
		ops = append(ops, diffOp{kind: opDel, line: a[i]})
		i++
	}
	for j < m {
		ops = append(ops, diffOp{kind: opAdd, line: b[j]})
		j++
	}
	return ops
}
