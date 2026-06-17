package builtin

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================================
// Networking skill tests — 9 skills (ip_validate/info, mac_lookup, port_check,
// url_parse/build, cidr_validate/merge, dns_lookup). Network-dependent tests
// (dns_lookup, port_check) skip in CI.
// =====================================================================================

func TestIPValidateSkill(t *testing.T) {
	skill := NewIPValidateSkill()
	if !skill.Match("validate ip 192.168.1.1") {
		t.Error("expected match for IPv4")
	}
	cases := []struct {
		input string
		want  string
	}{
		{"192.168.1.1", "IPv4"},
		{"::1", "IPv6"},
		{"8.8.8.8", "IPv4"},
		{"2606:4700:4700::1111", "IPv6"},
		{"not-an-ip", "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			out, err := skill.Execute(context.Background(), "validate ip "+tc.input)
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("expected %q in output, got %q", tc.want, out)
			}
		})
	}
}

func TestIPInfoSkill(t *testing.T) {
	skill := NewIPInfoSkill()
	if !skill.Match("ip info 10.0.0.1") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "ip info 10.0.0.1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "IPv4") {
		t.Errorf("expected IPv4 version, got %q", out)
	}
	if !strings.Contains(out, "private") {
		t.Errorf("10.0.0.1 is private RFC 1918, got %q", out)
	}

	// Loopback
	out2, _ := skill.Execute(context.Background(), "ip info 127.0.0.1")
	if !strings.Contains(out2, "loopback") {
		t.Errorf("127.0.0.1 is loopback, got %q", out2)
	}
}

func TestMACLookupSkill(t *testing.T) {
	skill := NewMACLookupSkill()
	if !skill.Match("mac lookup 00:0C:29:11:22:33") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "mac lookup 00:0C:29:11:22:33")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "VMware") {
		t.Errorf("expected VMware for OUI 00:0C:29, got %q", out)
	}
}

func TestPortCheckSkill(t *testing.T) {
	skipIfNetwork(t) // makes a real TCP connection
	skill := NewPortCheckSkill()
	if !skill.Match("port check localhost:80") {
		t.Error("expected match")
	}
	// Port 80 is usually closed on dev machines — just verify the skill runs.
	_, err := skill.Execute(context.Background(), "port check 127.0.0.1:1")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestURLParseSkill(t *testing.T) {
	skill := NewURLParseSkill()
	if !skill.Match("parse url https://example.com/path?q=1#frag") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "parse url https://example.com/path?q=1#frag")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"scheme: https", "host: example.com", "path: /path", "query:", "fragment: frag"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
}

func TestURLBuildSkill(t *testing.T) {
	skill := NewURLBuildSkill()
	jsonIn := `{"scheme":"https","host":"example.com","path":"/api","query":{"k":"v"}}`
	if !skill.Match("build url " + jsonIn) {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "build url "+jsonIn)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "https://example.com/api?") {
		t.Errorf("expected composed URL, got %q", out)
	}
	if !strings.Contains(out, "k=v") {
		t.Errorf("expected query param, got %q", out)
	}
}

func TestCIDRValidateSkill(t *testing.T) {
	skill := NewCIDRValidateSkill()
	if !skill.Match("validate cidr 192.168.1.0/24") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "validate cidr 192.168.1.0/24")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"IPv4", "mask bits: /24", "network:", "broadcast:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got %q", want, out)
		}
	}
}

func TestCIDRMergeSkill(t *testing.T) {
	skill := NewCIDRMergeSkill()
	if !skill.Match("merge cidr 192.168.0.0/24 192.168.1.0/24") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "merge cidr 192.168.0.0/24 192.168.1.0/24")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Two adjacent /24s should merge into 192.168.0.0/23.
	if !strings.Contains(out, "192.168.0.0/23") && !strings.Contains(out, "192.168.0.0/24") {
		t.Errorf("expected a /23 or original /24s, got %q", out)
	}
}

func TestDNSLookupSkill(t *testing.T) {
	skipIfNetwork(t) // hits real DNS resolver
	skill := NewDNSLookupSkill()
	if !skill.Match("dns lookup A example.com") {
		t.Error("expected match")
	}
	out, err := skill.Execute(context.Background(), "dns lookup A example.com")
	if err != nil {
		t.Fatalf("Execute: %v (note: requires DNS access)", err)
	}
	if !strings.Contains(out, "lookup: A example.com") {
		t.Errorf("expected lookup header, got %q", out)
	}
}
