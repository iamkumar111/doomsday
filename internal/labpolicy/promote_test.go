package labpolicy

import "testing"

func TestPromoteDraftToPolicy(t *testing.T) {
	p := &Policy{AllowedHosts: []string{"shop.example.com"}}
	d := &ReconDraft{
		TargetURL: "https://shop.example.com/",
		Host:      "shop.example.com",
		Combo:     "magento-abuse",
		L7Mode:    "catalog-search",
		Workers:   32,
		Streams:   200,
		BatchSize: 100,
	}
	PromoteDraftToPolicy(p, d)
	if p.Combo != "magento-abuse" || p.L7Mode != "catalog-search" || p.Workers != 32 {
		t.Fatalf("promote failed: %+v", p)
	}
	if !DraftHostAllowlisted(p, d) {
		t.Fatal("expected allowlisted host")
	}
}