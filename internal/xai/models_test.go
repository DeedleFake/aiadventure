package xai_test

import (
	"testing"

	"deedles.dev/aiadventure/internal/xai"
)

func TestEffortRequiredByModel(t *testing.T) {
	if !xai.EffortRequired("grok-4.5") {
		t.Fatal("grok-4.5 should support effort")
	}
	if xai.EffortRequired("grok-4.20-0309-non-reasoning") {
		t.Fatal("non-reasoning model should not require effort")
	}
	if xai.EffortRequired("grok-build-0.1") {
		t.Fatal("grok-build should not require effort")
	}
}

func TestValidEffort(t *testing.T) {
	if !xai.ValidEffort("grok-4.5", "high") {
		t.Fatal("high should be valid for grok-4.5")
	}
	if !xai.ValidEffort("grok-4.5", "") {
		t.Fatal("empty effort allowed (default applies)")
	}
	if xai.ValidEffort("grok-4.5", "xhigh") {
		t.Fatal("xhigh not valid for grok-4.5")
	}
	if !xai.ValidEffort("grok-build-0.1", "") {
		t.Fatal("empty ok for non-effort model")
	}
	if xai.ValidEffort("grok-build-0.1", "high") {
		t.Fatal("effort not allowed for grok-build")
	}
}

func TestResolveEffort(t *testing.T) {
	if got := xai.ResolveEffort("grok-4.5", ""); got != xai.EffortHigh {
		t.Fatalf("default grok-4.5 effort = %q, want high", got)
	}
	if got := xai.ResolveEffort("grok-4.5", "low"); got != xai.EffortLow {
		t.Fatalf("got %q", got)
	}
	if got := xai.ResolveEffort("grok-4.20-0309-reasoning", "high"); got != "" {
		t.Fatalf("non-configurable should omit effort, got %q", got)
	}
	if got := xai.ResolveEffort("grok-4.3", ""); got != xai.EffortLow {
		t.Fatalf("grok-4.3 default = %q", got)
	}
}

func TestBuildChatRequestIncludesEffortConditionally(t *testing.T) {
	msgs := []xai.Message{{Role: "user", Content: "hi"}}
	req := xai.BuildChatRequest("grok-4.5", "medium", msgs)
	if req.Model != "grok-4.5" || req.ReasoningEffort != "medium" {
		t.Fatalf("req = %+v", req)
	}
	req2 := xai.BuildChatRequest("grok-build-0.1", "high", msgs)
	if req2.ReasoningEffort != "" {
		t.Fatalf("should omit effort, got %q", req2.ReasoningEffort)
	}
	req3 := xai.BuildChatRequest("grok-4.5", "", msgs)
	if req3.ReasoningEffort != "high" {
		t.Fatalf("default effort = %q", req3.ReasoningEffort)
	}
}

func TestLookupCatalog(t *testing.T) {
	m, ok := xai.LookupModel("grok-4.5")
	if !ok || !m.SupportsEffort || len(m.EffortOptions) != 3 {
		t.Fatalf("model = %+v ok=%v", m, ok)
	}
	if _, ok := xai.LookupModel("nope"); ok {
		t.Fatal("expected miss")
	}
}
