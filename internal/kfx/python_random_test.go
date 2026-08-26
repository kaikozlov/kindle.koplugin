package kfx

import (
	"math"
	"testing"
)

func TestPythonRandomMatchesCPythonIntegerSeeds(t *testing.T) {
	// Generated with Python 3's random.Random(seed).random(). These pin the
	// CPython MT19937 integer-seed path used by kfxlib Scribe density strokes.
	tests := []struct {
		seed uint64
		want []float64
	}{
		{0, []float64{0.84442185152504812, 0.75795440294030247, 0.420571580830845, 0.25891675029296335, 0.51127472136860852}},
		{1, []float64{0.13436424411240122, 0.84743373693723267, 0.76377461897661403, 0.2550690257394217, 0.49543508709194095}},
		{42, []float64{0.63942679845788375, 0.025010755222666936, 0.27502931836911926, 0.22321073814882275, 0.7364712141640124}},
		{123456789, []float64{0.64140061618587263, 0.54218926809694945, 0.99317506628327212, 0.84325213668691656, 0.81173392833794056}},
		{1099511627783, []float64{0.61370377799365106, 0.81491629733094872, 0.9450115087592873, 0.57039069910045614, 0.47592409887048936}},
	}
	for _, tc := range tests {
		r := newPythonRandomSeed(tc.seed)
		for i, want := range tc.want {
			got := r.Float64()
			if math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("seed %d sample %d = %.17g, want %.17g", tc.seed, i, got, want)
			}
		}
	}
}
