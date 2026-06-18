package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Converter skill tests — csv_to_markdown_table, markdown_table_to_csv,
// json_to_markdown_table, tsv_to_csv, csv_to_tsv, list_to_markdown.
// Uses the shared runSkillCases driver from encoding_test.go.
// =====================================================================================

// ---- csv_to_markdown_table ----

func TestCSVToMarkdown(t *testing.T) {
	runSkillCases(t, "csv_to_markdown_table", NewCSVToMarkdownTableSkill(), []skillCase{
		{
			name:        "positive: basic header + row",
			query:       "csv to markdown table:\nname,age\nAlice,30",
			shouldMatch: true,
			want:        "| name | age |\n| --- | --- |\n| Alice | 30 |",
			wantErr:     false,
		},
		{
			name:        "positive: alias csv to markdown",
			query:       "convert csv to markdown:\na,b\n1,2",
			shouldMatch: true,
			want:        "| a | b |\n| --- | --- |\n| 1 | 2 |",
			wantErr:     false,
		},
		{
			name:        "positive: cell with comma is quoted in source CSV",
			query:       "csv to markdown table:\nname,note\n\"Doe, John\",hi",
			shouldMatch: true,
			want:        "| name | note |\n| --- | --- |\n| Doe, John | hi |",
			wantErr:     false,
		},
		{
			name:        "negative: unrelated query",
			query:       "csv to json: a,b\n1,2",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: empty input",
			query:       "csv to markdown table:",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- markdown_table_to_csv ----

func TestMarkdownToCSV(t *testing.T) {
	runSkillCases(t, "markdown_table_to_csv", NewMarkdownTableToCSVSkill(), []skillCase{
		{
			name:        "positive: round-trip a basic table",
			query:       "markdown table to csv:\n| name | age |\n| --- | --- |\n| Alice | 30 |",
			shouldMatch: true,
			want:        "name,age\nAlice,30",
			wantErr:     false,
		},
		{
			name:        "positive: alias markdown to csv",
			query:       "markdown to csv:\n| a | b |\n| --- | --- |\n| 1 | 2 |",
			shouldMatch: true,
			want:        "a,b\n1,2",
			wantErr:     false,
		},
		{
			name: "positive: cell with comma gets quoted on output",
			query: "markdown to csv:\n| name | note |\n| --- | --- |\n| Doe, John | hi |",
			shouldMatch: true,
			want:        "\"Doe, John\",hi",
			wantErr:     false,
		},
		{
			name:        "negative: not a markdown query",
			query:       "csv to markdown: a,b\n1,2",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: no table rows in input",
			query:       "markdown to csv: just some plain text",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- json_to_markdown_table ----

func TestJSONToMarkdown(t *testing.T) {
	runSkillCases(t, "json_to_markdown_table", NewJSONToMarkdownTableSkill(), []skillCase{
		{
			name:        "positive: flat array",
			query:       "json to markdown table:\n[{\"name\":\"Alice\",\"age\":30},{\"name\":\"Bob\",\"age\":25}]",
			shouldMatch: true,
			want:        "| age | name |\n| --- | --- |\n| 30 | Alice |\n| 25 | Bob |",
			wantErr:     false,
		},
		{
			name:        "positive: alias json to markdown",
			query:       "convert json to markdown:\n[{\"a\":1,\"b\":2}]",
			shouldMatch: true,
			want:        "| a | b |\n| --- | --- |\n| 1 | 2 |",
			wantErr:     false,
		},
		{
			name:        "positive: nested value rendered as JSON",
			query:       "json to markdown table:\n[{\"name\":\"x\",\"meta\":{\"k\":\"v\"}}]",
			shouldMatch: true,
			want:        "{\"k\":\"v\"}",
			wantErr:     false,
		},
		{
			name:        "negative: csv query",
			query:       "csv to json:\na,b",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: empty array",
			query:       "json to markdown table:\n[]",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
		{
			name:        "edge: malformed JSON",
			query:       "json to markdown table:\n{not json}",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- tsv_to_csv ----

func TestTSVToCSV(t *testing.T) {
	runSkillCases(t, "tsv_to_csv", NewTSVToCSVSkill(), []skillCase{
		{
			name:        "positive: simple TSV",
			query:       "tsv to csv:\nname\tage\nAlice\t30",
			shouldMatch: true,
			want:        "name,age\nAlice,30",
			wantErr:     false,
		},
		{
			name:        "positive: field containing comma is quoted",
			query:       "tsv to csv:\nname\tcity\nDoe, John\tNYC",
			shouldMatch: true,
			want:        "\"Doe, John\",NYC",
			wantErr:     false,
		},
		{
			name:        "positive: alias convert tsv to csv",
			query:       "convert tsv to csv:\na\tb\n1\t2",
			shouldMatch: true,
			want:        "a,b\n1,2",
			wantErr:     false,
		},
		{
			name:        "negative: csv to tsv query",
			query:       "csv to tsv:\na,b\n1,2",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: empty input",
			query:       "tsv to csv:",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- csv_to_tsv ----

func TestCSVToTSV(t *testing.T) {
	runSkillCases(t, "csv_to_tsv", NewCSVToTSVSkill(), []skillCase{
		{
			name:        "positive: simple CSV",
			query:       "csv to tsv:\nname,age\nAlice,30",
			shouldMatch: true,
			want:        "name\tage\nAlice\t30",
			wantErr:     false,
		},
		{
			name:        "positive: quoted field with comma collapses to single tab-delim cell",
			query:       "csv to tsv:\nname,note\n\"Doe, John\",hi",
			shouldMatch: true,
			want:        "Doe, John\thi",
			wantErr:     false,
		},
		{
			name:        "positive: alias convert csv to tsv",
			query:       "convert csv to tsv:\na,b\n1,2",
			shouldMatch: true,
			want:        "a\tb\n1\t2",
			wantErr:     false,
		},
		{
			name:        "negative: tsv to csv query",
			query:       "tsv to csv:\na\tb",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: empty input",
			query:       "csv to tsv:",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- list_to_markdown ----

func TestListToMarkdown(t *testing.T) {
	runSkillCases(t, "list_to_markdown", NewListToMarkdownSkill(), []skillCase{
		{
			name:        "positive: bullet list",
			query:       "list to markdown:\napple\nbanana\ncherry",
			shouldMatch: true,
			want:        "- apple\n- banana\n- cherry",
			wantErr:     false,
		},
		{
			name:        "positive: numbered list",
			query:       "list to markdown numbered:\nfirst\nsecond\nthird",
			shouldMatch: true,
			want:        "1. first\n2. second\n3. third",
			wantErr:     false,
		},
		{
			name:        "positive: alias to markdown list",
			query:       "to markdown list:\none\ntwo",
			shouldMatch: true,
			want:        "- one\n- two",
			wantErr:     false,
		},
		{
			name:        "positive: ordered keyword also produces numbered output",
			query:       "list to markdown ordered:\nx\ny",
			shouldMatch: true,
			want:        "1. x\n2. y",
			wantErr:     false,
		},
		{
			name:        "positive: existing markers are re-normalized",
			query:       "list to markdown:\n- alpha\n* beta",
			shouldMatch: true,
			want:        "- alpha\n- beta",
			wantErr:     false,
		},
		{
			name:        "negative: unrelated query",
			query:       "csv to markdown:\na,b",
			shouldMatch: false,
			want:        "",
			wantErr:     false,
		},
		{
			name:        "edge: empty input",
			query:       "list to markdown:",
			shouldMatch: true,
			want:        "",
			wantErr:     true,
		},
	})
}

// ---- round-trip sanity check ----

// TestConverterRoundTrip exercises csv -> markdown -> csv to make sure the
// pair stays consistent for a representative input.
func TestConverterRoundTrip(t *testing.T) {
	ctx := context.Background()
	orig := "name,age\nAlice,30\nBob,25"
	md, err := NewCSVToMarkdownTableSkill().Execute(ctx, "csv to markdown table:\n"+orig)
	if err != nil {
		t.Fatalf("csv -> markdown: %v", err)
	}
	if !strings.Contains(md, "| Alice | 30 |") {
		t.Errorf("markdown output missing expected row: %q", md)
	}
	back, err := NewMarkdownTableToCSVSkill().Execute(ctx, "markdown to csv:\n"+md)
	if err != nil {
		t.Fatalf("markdown -> csv: %v", err)
	}
	if !strings.Contains(back, "Alice,30") {
		t.Errorf("round-trip csv missing expected row: %q", back)
	}
	if !strings.Contains(back, "Bob,25") {
		t.Errorf("round-trip csv missing expected row: %q", back)
	}
}
