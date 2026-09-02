package permute

import "testing"

func TestQueueModeUniformAndMix(t *testing.T) {
	u := Cell{ID: "binary|none|tween|bicameral|simd|cs=0.1|lr=0.05", Mode: ModeTween}
	if g := QueueMode(u); g != "tween" {
		t.Fatalf("uniform %q want tween", g)
	}
	m := Cell{
		ID:   "binary|none|sgd|bicameral|simd|bm=tween+sgd|pat=alt|cs=0.1|lr=0.05",
		Mode: ModeSGD,
	}
	if g := QueueMode(m); g != "cam-mix" {
		t.Fatalf("mix %q want cam-mix", g)
	}
	bm, pat, ok := MixTagsFromID(m.ID)
	if !ok || pat != "alt" || len(bm) != 2 || bm[0] != "tween" || bm[1] != "sgd" {
		t.Fatalf("tags bm=%v pat=%q ok=%v", bm, pat, ok)
	}
}
