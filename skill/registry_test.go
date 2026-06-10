package skill

import (
	"context"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return "test", nil
	}

	err := r.Register("test", `(?i)test`, handler, "Test skill")
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if r.Count() != 1 {
		t.Errorf("Expected 1 skill, got %d", r.Count())
	}
}

func TestMatch(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return input, nil
	}

	r.Register("greet", `(?i)hello\s+(\w+)`, handler, "Greet skill")

	skill, found := r.Match("hello world")
	if !found {
		t.Fatal("Match failed to find skill")
	}

	if skill.Name != "greet" {
		t.Errorf("Expected skill name 'greet', got '%s'", skill.Name)
	}
}

func TestExecute(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return "echo: " + input, nil
	}

	r.Register("echo", `echo\s+(.+)`, handler, "Echo skill")

	ctx := context.Background()
	result, found, err := r.Execute(ctx, "echo test")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !found {
		t.Fatal("Execute did not find a match")
	}
	if result != "echo: echo test" {
		t.Errorf("Unexpected result: %s", result)
	}
}

func TestList(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return "", nil
	}

	r.Register("skill1", `pattern1`, handler, "desc")
	r.Register("skill2", `pattern2`, handler, "desc")

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("Expected 2 skills, got %d", len(names))
	}
}

func TestGet(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return "", nil
	}

	r.Register("test", `test`, handler, "Test skill")

	skill, ok := r.Get("test")
	if !ok {
		t.Fatal("Get failed to find skill")
	}
	if skill.Name != "test" {
		t.Errorf("Expected skill name 'test', got '%s'", skill.Name)
	}
}

func TestUnregister(t *testing.T) {
	r := NewRegistry()
	handler := func(ctx context.Context, input string) (string, error) {
		return "", nil
	}

	r.Register("test", `test`, handler, "Test skill")
	if r.Count() != 1 {
		t.Errorf("Expected 1 skill, got %d", r.Count())
	}

	r.Unregister("test")
	if r.Count() != 0 {
		t.Errorf("Expected 0 skills after unregister, got %d", r.Count())
	}
}

func TestRegisterBuiltinSkills(t *testing.T) {
	r := NewRegistry()
	err := RegisterBuiltinSkills(r)
	if err != nil {
		t.Fatalf("RegisterBuiltinSkills failed: %v", err)
	}

	if r.Count() != 8 {
		t.Errorf("Expected 8 builtin skills, got %d", r.Count())
	}

	names := r.List()
	expected := []string{"math", "time", "hash", "encode", "uuid", "json_format", "health", "unit_convert"}
	for _, exp := range expected {
		found := false
		for _, name := range names {
			if name == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected builtin skill '%s' not found", exp)
		}
	}
}

func TestGenerateSkill(t *testing.T) {
	r := NewRegistry()
	err := r.GenerateSkill("echo_test", `echo\s+(.+)`, "You said: {0}")
	if err != nil {
		t.Fatalf("GenerateSkill failed: %v", err)
	}

	ctx := context.Background()
	result, found, err := r.Execute(ctx, "echo hello world")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !found {
		t.Fatal("Execute did not find a match")
	}
	expected := "You said: hello world"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}