package main

import (
	"math"
	"testing"
)

func TestParseConcurrency(t *testing.T) {
	cases := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{in: "1,4,16", want: []int{1, 4, 16}},
		{in: " 1 , 4 ", want: []int{1, 4}},
		{in: "1,0", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "", wantErr: true},
		{in: ",,", wantErr: true},
	}
	for _, c := range cases {
		got, err := parseConcurrency(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseConcurrency(%q): expected error, got %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseConcurrency(%q): unexpected error %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseConcurrency(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseConcurrency(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestPercentile(t *testing.T) {
	sorted := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if got := percentile(sorted, 0.50); got != 5 {
		t.Errorf("p50 = %v, want 5", got)
	}
	if got := percentile(sorted, 0.95); got != 10 {
		t.Errorf("p95 = %v, want 10", got)
	}
	if got := percentile(sorted, 0.00); got != 1 {
		t.Errorf("p0 = %v, want 1", got)
	}
	if got := percentile([]int64{}, 0.95); got != 0 {
		t.Errorf("empty p95 = %v, want 0", got)
	}
}

func TestSummarize(t *testing.T) {
	s := summarize([]int64{10, 20, 30, 40})
	if s.Min != 10 || s.Max != 40 {
		t.Errorf("min/max = %v/%v, want 10/40", s.Min, s.Max)
	}
	if math.Abs(s.Mean-25) > 1e-9 {
		t.Errorf("mean = %v, want 25", s.Mean)
	}
	// Nearest-rank median of an even-length sample is the lower-middle value.
	if s.P50 != 20 {
		t.Errorf("p50 = %v, want 20", s.P50)
	}

	empty := summarize(nil)
	if empty.Min != 0 || empty.Mean != 0 || empty.P50 != 0 || empty.P95 != 0 || empty.P99 != 0 || empty.Max != 0 {
		t.Errorf("summarize(nil) = %+v, want all zero", empty)
	}
}
