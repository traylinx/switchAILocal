package switchailocal

import "testing"

func TestApplyModelPrefixesHidesPrefixedAliasesFromPublicCatalog(t *testing.T) {
	models := []*ModelInfo{{
		ID:         "ail-fast",
		Object:     "model",
		OwnedBy:    "switchai",
		Visibility: "",
	}}

	got := applyModelPrefixes(models, "deepseek", false)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].ID != "ail-fast" || got[0].Visibility == "private" {
		t.Fatalf("base alias mutated: %#v", got[0])
	}
	if got[1].ID != "deepseek/ail-fast" {
		t.Fatalf("prefixed id=%q", got[1].ID)
	}
	if got[1].Visibility != "private" {
		t.Fatalf("prefixed alias visibility=%q want private", got[1].Visibility)
	}
}

func TestApplyModelPrefixesForceKeepsOnlyPrefixedPublicAlias(t *testing.T) {
	models := []*ModelInfo{{ID: "ail-fast", Object: "model", OwnedBy: "switchai"}}

	got := applyModelPrefixes(models, "deepseek", true)
	if len(got) != 1 {
		t.Fatalf("len=%d want 1", len(got))
	}
	if got[0].ID != "deepseek/ail-fast" {
		t.Fatalf("id=%q", got[0].ID)
	}
	if got[0].Visibility == "private" {
		t.Fatal("forceModelPrefix should not hide the only emitted prefixed alias")
	}
}
