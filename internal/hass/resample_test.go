package hass

import (
	"math"
	"testing"
	"time"
)

func TestBuildTimestamps_EvenSpacing(t *testing.T) {
	end := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	timestamps := BuildTimestamps(end, 24*time.Hour, 5)

	if len(timestamps) != 5 {
		t.Fatalf("len(timestamps) = %d, want 5", len(timestamps))
	}
	wantFirst := end.Add(-24 * time.Hour)
	if !timestamps[0].Equal(wantFirst) {
		t.Errorf("timestamps[0] = %v, want %v", timestamps[0], wantFirst)
	}
	if !timestamps[4].Equal(end) {
		t.Errorf("timestamps[4] = %v, want %v", timestamps[4], end)
	}
	wantStep := 6 * time.Hour
	if gotStep := timestamps[1].Sub(timestamps[0]); gotStep != wantStep {
		t.Errorf("step = %v, want %v", gotStep, wantStep)
	}
}

func TestBuildTimestamps_SinglePoint(t *testing.T) {
	end := time.Date(2026, 7, 10, 20, 0, 0, 0, time.UTC)
	timestamps := BuildTimestamps(end, time.Hour, 1)
	if len(timestamps) != 1 || !timestamps[0].Equal(end) {
		t.Errorf("timestamps = %v, want [end]", timestamps)
	}
}

func TestStepForwardFill_CarriesLastKnownValue(t *testing.T) {
	base := time.Date(2026, 7, 10, 8, 0, 0, 0, time.UTC)
	points := []HistoryPoint{
		{Time: base, Value: 20.0},
		{Time: base.Add(2 * time.Hour), Value: 22.0},
	}
	timestamps := []time.Time{
		base.Add(-1 * time.Hour), // before first point -> falls back to first value
		base.Add(1 * time.Hour),  // between points -> carries 20.0 forward
		base.Add(3 * time.Hour),  // after second point -> carries 22.0 forward
	}

	values := StepForwardFill(points, timestamps)

	want := []float64{20.0, 20.0, 22.0}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("values[%d] = %v, want %v", i, values[i], want[i])
		}
	}
}

func TestStepForwardFill_EmptyPointsReturnsNaN(t *testing.T) {
	timestamps := []time.Time{time.Now(), time.Now().Add(time.Hour)}
	values := StepForwardFill(nil, timestamps)

	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}
	for i, v := range values {
		if !math.IsNaN(v) {
			t.Errorf("values[%d] = %v, want NaN", i, v)
		}
	}
}

func TestAverageSeries_ElementwiseAverage(t *testing.T) {
	series := [][]float64{
		{10, 20, 30},
		{20, 30, 40},
	}
	avg := AverageSeries(series)

	want := []float64{15, 25, 35}
	for i := range want {
		if avg[i] != want[i] {
			t.Errorf("avg[%d] = %v, want %v", i, avg[i], want[i])
		}
	}
}

func TestAverageSeries_SkipsNaN(t *testing.T) {
	series := [][]float64{
		{10, math.NaN(), 30},
		{20, 25, 40},
	}
	avg := AverageSeries(series)

	if avg[0] != 15 {
		t.Errorf("avg[0] = %v, want 15", avg[0])
	}
	if avg[1] != 25 {
		t.Errorf("avg[1] = %v, want 25 (only non-NaN value)", avg[1])
	}
	if avg[2] != 35 {
		t.Errorf("avg[2] = %v, want 35", avg[2])
	}
}

func TestAverageSeries_AllNaNProducesNaN(t *testing.T) {
	series := [][]float64{
		{math.NaN()},
		{math.NaN()},
	}
	avg := AverageSeries(series)
	if !math.IsNaN(avg[0]) {
		t.Errorf("avg[0] = %v, want NaN", avg[0])
	}
}
