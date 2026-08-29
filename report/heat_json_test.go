package report

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestHeatMarshalJSONAllowsNaNGaps(t *testing.T) {
	h := Heat{
		Modes:          []string{"sgd", "tween"},
		DTypes:         []string{"f32", "bin"},
		ModeDTypeScore: [][]float64{{1.5, math.NaN()}, {math.NaN(), 2.25}},
		ModeMeanScore:  []float64{1.5, math.NaN()},
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, "null") {
		t.Fatalf("expected null for NaN gaps, got %s", s)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	grid := raw["mode_dtype_score"].([]any)
	row0 := grid[0].([]any)
	if row0[1] != nil {
		t.Fatalf("want null at [0][1], got %#v", row0[1])
	}
}
