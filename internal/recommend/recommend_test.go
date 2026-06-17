package recommend

import (
	"testing"

	"github.com/metabbe3/Kyoci-Agent/internal/hardware"
)

func TestRecommend_Boundaries(t *testing.T) {
	cases := []struct {
		name     string
		ramGB    int
		wantBest string
	}{
		{"tiny", 4, ""},        // no tier fits comfortably
		{"8gb", 8, "qwen2.5:3b"},
		{"16gb", 16, "llama3.1:8b"},
		{"32gb", 32, "qwen2.5:14b"},
		{"64gb", 64, "qwen2.5:32b"},
		{"96gb", 96, "llama3.3:70b"},
		{"128gb", 128, "llama3.3:70b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Recommend(&hardware.Specs{RAMGB: tc.ramGB, OS: "darwin", Arch: "arm64"})
			var gotBest string
			for _, p := range res.Local {
				if p.Recommended {
					gotBest = p.Model
				}
			}
			if gotBest != tc.wantBest {
				t.Fatalf("RAM=%dGB: recommended=%q want %q", tc.ramGB, gotBest, tc.wantBest)
			}
		})
	}
}

func TestRecommend_NilSpecs(t *testing.T) {
	res := Recommend(nil)
	if !res.Cloud.Needed {
		t.Fatal("nil specs should trigger cloud advice")
	}
	if len(res.Local) == 0 {
		t.Fatal("nil specs should still return tier list (all too_big)")
	}
}

func TestRecommend_CloudAdvice(t *testing.T) {
	cases := []struct {
		ramGB     int
		wantNeed  bool
	}{
		{4, true},
		{8, true},  // 8-16 → cloud still recommended
		{16, true},
		{32, false}, // 32+ → cloud optional
		{64, false},
	}
	for _, tc := range cases {
		adv := cloudAdvice(tc.ramGB, true)
		if adv.Needed != tc.wantNeed {
			t.Errorf("RAM=%dGB: cloud needed=%v want %v (%s)", tc.ramGB, adv.Needed, tc.wantNeed, adv.Summary)
		}
	}
}

func TestClassify(t *testing.T) {
	tier := Tier{MinRAM: 16, Model: "x"}
	cases := []struct {
		ram       int
		wantVerdt string
	}{
		{8, "too_big"},
		{12, "slow"},  // 16*3/4 = 12
		{16, "tight"},
		{32, "fits"},  // 16*2 = 32
	}
	for _, tc := range cases {
		got, _ := classify(tier, tc.ram)
		if got != tc.wantVerdt {
			t.Errorf("RAM=%d: verdict=%q want %q", tc.ram, got, tc.wantVerdt)
		}
	}
}
