package river

import (
	"net/http"
	"testing"
)

func TestBuildLeanBarsRanksSmallThenFast(t *testing.T) {
	rows := []Row{
		{ID: "champ", Acc: 70, WeightBytes: 200 * 1024, WeightKiB: 200, DurationSec: 10, Mode: "sgd", DType: "f32", Arch: "single"},
		{ID: "big-fast", Acc: 68, WeightBytes: 500 * 1024, WeightKiB: 500, DurationSec: 2, Mode: "sgd", DType: "f16", Arch: "single"},
		{ID: "small-slow", Acc: 67, WeightBytes: 50 * 1024, WeightKiB: 50, DurationSec: 20, Mode: "MeshBP", DType: "int8", Arch: "cameral×4"},
		{ID: "small-fast", Acc: 66.6, WeightBytes: 50 * 1024, WeightKiB: 50, DurationSec: 5, Mode: "MeshBP", DType: "int4", Arch: "cameral×4"},
		{ID: "miss", Acc: 50, WeightBytes: 10 * 1024, WeightKiB: 10, DurationSec: 1, Mode: "tween", DType: "nf4", Arch: "single"},
	}
	sum, bars := buildLeanBars(rows, 20)
	if sum.BestAcc != 70 || sum.Threshold < 66.4 || sum.Threshold > 66.6 {
		t.Fatalf("summary %+v", sum)
	}
	if sum.Eligible != 4 { // miss is below 95% of 70
		t.Fatalf("eligible %d", sum.Eligible)
	}
	if len(bars) < 2 {
		t.Fatalf("bars %d", len(bars))
	}
	// smallest RAM first; among 50KiB, faster duration wins
	if bars[0].ID != "small-fast" {
		t.Fatalf("winner %s want small-fast", bars[0].ID)
	}
	if bars[1].ID != "small-slow" {
		t.Fatalf("runner-up %s want small-slow", bars[1].ID)
	}
}

func TestBuildLeanByDtypeSmallToBig(t *testing.T) {
	rows := []Row{
		{ID: "champ", Acc: 70, WeightBytes: 200 * 1024, WeightKiB: 200, DurationSec: 10, Mode: "sgd", DType: "float32", Arch: "single"},
		{ID: "i8-big", Acc: 68, WeightBytes: 80 * 1024, WeightKiB: 80, DurationSec: 5, Mode: "sgd", DType: "int8", Arch: "cameral×4"},
		{ID: "i8-small", Acc: 67, WeightBytes: 40 * 1024, WeightKiB: 40, DurationSec: 8, Mode: "MeshBP", DType: "int8", Arch: "cameral×5"},
		{ID: "u16", Acc: 67, WeightBytes: 60 * 1024, WeightKiB: 60, DurationSec: 6, Mode: "tween_chain", DType: "uint16", Arch: "cameral×6"},
		{ID: "miss", Acc: 50, WeightBytes: 10 * 1024, WeightKiB: 10, DurationSec: 1, Mode: "tween", DType: "nf4", Arch: "single"},
	}
	_, bars := buildLeanByDtype(rows)
	if len(bars) != 3 { // int8, uint16, float32 — nf4 below threshold
		t.Fatalf("bars %d %+v", len(bars), bars)
	}
	if bars[0].DType != "int8" || bars[0].ID != "i8-small" {
		t.Fatalf("first %+v want int8/i8-small", bars[0])
	}
	if bars[1].DType != "uint16" || bars[2].DType != "float32" {
		t.Fatalf("order %+v %+v", bars[1], bars[2])
	}
}

func TestPublicURLForRequestUsesBrowserHost(t *testing.T) {
	r := &http.Request{Host: "192.168.0.22:8177"}
	got := PublicURLForRequest(r, "8203")
	want := "http://192.168.0.22:8203"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestBuildNearKeepFloor(t *testing.T) {
	rows := []Row{
		{ID: "champ", Acc: 100, SoftAcc: 90, Throughput: 50, Availability: 80, WeightBytes: 200 * 1024, WeightKiB: 200, DurationSec: 10, Mode: "sgd", DType: "f32", Arch: "single", LRLabel: "0.6", Status: "ok"},
		{ID: "keep95", Acc: 95, SoftAcc: 88, Throughput: 80, Availability: 90, WeightBytes: 40 * 1024, WeightKiB: 40, DurationSec: 4, Mode: "MeshBP", DType: "int8", Arch: "cameral×4", LRLabel: "0.6", Status: "ok"},
		{ID: "chain", Acc: 92, SoftAcc: 85, Throughput: 70, Availability: 85, WeightBytes: 30 * 1024, WeightKiB: 30, DurationSec: 3, Mode: "tween_chain", DType: "int8", Arch: "cameral×4", LRLabel: "0.6", Status: "ok"},
		{ID: "nonbp", Acc: 88, SoftAcc: 80, Throughput: 120, Availability: 95, WeightBytes: 25 * 1024, WeightKiB: 25, DurationSec: 2, Mode: "tween", DType: "int4", Arch: "cameral×5", LRLabel: "0.6", Status: "ok"},
		{ID: "keep70", Acc: 70, SoftAcc: 60, Throughput: 100, Availability: 95, WeightBytes: 20 * 1024, WeightKiB: 20, DurationSec: 2, Mode: "TweenSplit", DType: "int4", Arch: "cameral×5", LRLabel: "0.6", Status: "ok"},
		{ID: "miss", Acc: 50, SoftAcc: 40, Throughput: 200, Availability: 99, WeightBytes: 10 * 1024, WeightKiB: 10, DurationSec: 1, Mode: "sgd", DType: "nf4", Arch: "single", LRLabel: "0.6", Status: "ok"},
	}
	for i := range rows {
		enrichRow(&rows[i])
	}
	p := buildNear(File{Rows: rows}, 0.70)
	if p.BestAcc != 100 || p.NBand != 5 {
		t.Fatalf("best=%v n_band=%d want 100/5", p.BestAcc, p.NBand)
	}
	if p.BestNonBP == nil || p.BestNonBP.ID != "nonbp" {
		t.Fatalf("best non-BP %+v want nonbp", p.BestNonBP)
	}
	if p.BestChain == nil || p.BestChain.ID != "chain" || p.BestChain.Note == "" {
		t.Fatalf("best chain %+v", p.BestChain)
	}
	if creditOfMode("tween_chain") != creditChain || creditOfMode("MeshTweenChain") != creditChain {
		t.Fatalf("chain credit misclassified")
	}
	if creditOfMode("sgd") != creditBP || creditOfMode("tween") != creditNonBP {
		t.Fatalf("bp/non-bp misclassified")
	}
	var marked bool
	for _, r := range p.Rows {
		if r.ID == "nonbp" && r.BestNonBP {
			marked = true
		}
		if r.ID == "chain" && r.Credit != creditChain {
			t.Fatalf("chain row credit %s", r.Credit)
		}
	}
	if !marked {
		t.Fatal("best non-BP row not flagged")
	}
}

func TestBuildLPDSearchRanksByLPD(t *testing.T) {
	rows := []Row{
		{ID: "champ", Acc: 100, SoftAcc: 90, Throughput: 50, Availability: 80, WeightBytes: 200 * 1024, WeightKiB: 200, DurationSec: 10, Mode: "sgd", DType: "f32", Arch: "single", LRLabel: "0.6", Status: "ok"},
		{ID: "dense", Acc: 95, SoftAcc: 88, Throughput: 80, Availability: 90, WeightBytes: 40 * 1024, WeightKiB: 40, DurationSec: 4, Mode: "MeshBP", DType: "int8", Arch: "cameral×4", LRLabel: "0.6", Status: "ok"},
		{ID: "trap", Acc: 20, SoftAcc: 10, Throughput: 500, Availability: 99, WeightBytes: 5 * 1024, WeightKiB: 5, DurationSec: 1, Mode: "sgd", DType: "nf4", Arch: "single", LRLabel: "0.6", Status: "ok"},
	}
	p := buildLPDSearch(File{Rows: rows})
	if p.BestAcc != 100 || p.NKeep < 2 {
		t.Fatalf("best=%v n_keep=%d", p.BestAcc, p.NKeep)
	}
	if len(p.Rows) != 3 {
		t.Fatalf("want all 3 rows incl trap, got %d", len(p.Rows))
	}
	if p.TopLPD == nil || p.TopLPD.ID == "trap" {
		t.Fatalf("top LPD should not be trap: %+v", p.TopLPD)
	}
	if p.Rows[0].LPD < p.Rows[len(p.Rows)-1].LPD && p.Rows[len(p.Rows)-1].LPD > 0 {
		t.Fatalf("rows not sorted by LPD desc")
	}
}

func TestComparePDFSmoke(t *testing.T) {
	rows := []Row{
		{ID: "a", Acc: 70, WeightBytes: 1000, WeightKiB: 1, DurationSec: 3, Mode: "sgd", DType: "int8", Arch: "single", Throughput: 100, AccPerSec: 2},
		{ID: "b", Acc: 68, WeightBytes: 2000, WeightKiB: 2, DurationSec: 4, Mode: "MeshBP", DType: "float32", Arch: "cameral×4", Throughput: 90, AccPerSec: 1.5},
	}
	for i := range rows {
		enrichRow(&rows[i])
	}
	payload := buildCompare(File{Machine: "test", Matrix: "sprint", Rows: rows, LRLabels: []string{"0.6"}})
	near := buildNear(File{Rows: rows}, 0)
	lpd := buildLPDSearch(File{Rows: rows})
	thru := buildThru(File{Rows: rows})
	pdf, err := ComparePDF(payload, near, lpd, thru, PlanProgress{Plan: 10, Done: 2, Left: 8, Pct: 20, ETAHuman: "1h"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) < 100 || string(pdf[:4]) != "%PDF" {
		t.Fatalf("bad pdf len=%d", len(pdf))
	}
}

func TestApplyMixTags(t *testing.T) {
	r := Row{
		ID:   "binary|none|NormalBP|bicameral|simd|lr=0.6|bm=tween+sgd|pat=alt",
		Mode: "NormalBP",
	}
	enrichRow(&r)
	if r.ParentMode != "NormalBP" {
		t.Fatalf("parent %q", r.ParentMode)
	}
	if r.MixPattern != "alt" || len(r.BranchModes) != 2 {
		t.Fatalf("tags %+v %v", r.MixPattern, r.BranchModes)
	}
	if r.Mode != "alt · tween+sgd" || r.MixLabel != r.Mode {
		t.Fatalf("mode %q label %q", r.Mode, r.MixLabel)
	}
}

func TestApplyMixTagsCamSync(t *testing.T) {
	r := Row{
		ID:   "binary|none|NormalBP|bicameral|simd|lr=0.6|bm=tween+sgd|pat=alt|cs=0.10",
		Mode: "NormalBP",
	}
	enrichRow(&r)
	if r.Mode != "alt · tween+sgd · cs=10%" {
		t.Fatalf("mode %q", r.Mode)
	}
}

func TestBuildThruRanksByThroughput(t *testing.T) {
	p := buildThru(File{Rows: []Row{
		{ID: "slow", Acc: 90, Throughput: 50, Availability: 80, WeightBytes: 100 * 1024, Mode: "sgd", DType: "f32", Arch: "single", Status: "ok"},
		{ID: "fast", Acc: 80, Throughput: 200, Availability: 90, WeightBytes: 40 * 1024, Mode: "tween", DType: "int8", Arch: "cameral×4", Status: "ok"},
	}})
	if p.NRows != 2 || p.BestID != "fast" || p.Rows[0].ID != "fast" {
		t.Fatalf("thru %+v rows=%v", p.BestID, p.Rows)
	}
}

func TestEnrichRestoresModeFromID(t *testing.T) {
	r := Row{
		ID:   "int64|none|tween|cameral×5|simd|lr=0.6",
		Mode: "NormalBP", // poisoned
	}
	enrichRow(&r)
	if r.Mode != "tween" {
		t.Fatalf("mode %q want tween", r.Mode)
	}
	if creditOfRow(r) != creditNonBP {
		t.Fatalf("credit %s", creditOfRow(r))
	}
}

func TestPulseTrainModeUniformNotNormalBP(t *testing.T) {
	r := Row{ID: "int16|none|MeshTweenChain|cameral×5|simd|lr=0.6", Mode: "NormalBP"}
	if got := pulseTrainMode(r); got != "MeshTweenChain" {
		t.Fatalf("got %q", got)
	}
	mix := Row{
		ID: "binary|none|NormalBP|bicameral|simd|lr=0.6|bm=tween+sgd|pat=alt",
		Mode: "alt · tween+sgd", ParentMode: "NormalBP",
		BranchModes: []string{"tween", "sgd"}, MixPattern: "alt",
	}
	if got := pulseTrainMode(mix); got != "NormalBP" {
		t.Fatalf("mix parent %q", got)
	}
}
