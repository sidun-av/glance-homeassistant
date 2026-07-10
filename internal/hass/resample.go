package hass

import (
	"math"
	"sort"
	"time"
)

func BuildTimestamps(end time.Time, rangeDur time.Duration, maxPoints int) []time.Time {
	if maxPoints < 2 {
		return []time.Time{end}
	}
	start := end.Add(-rangeDur)
	step := rangeDur / time.Duration(maxPoints-1)
	timestamps := make([]time.Time, maxPoints)
	for i := 0; i < maxPoints; i++ {
		timestamps[i] = start.Add(step * time.Duration(i))
	}
	return timestamps
}

// StepForwardFill resamples irregular history points onto evenly spaced
// timestamps: the value at each timestamp is the most recently known state
// at or before it. Timestamps before the first known point fall back to the
// first point's value, so a room's history never has a gap at the start of
// the window just because the entity's first ever state came slightly later.
func StepForwardFill(points []HistoryPoint, timestamps []time.Time) []float64 {
	values := make([]float64, len(timestamps))
	if len(points) == 0 {
		for i := range values {
			values[i] = math.NaN()
		}
		return values
	}

	sorted := make([]HistoryPoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })

	idx := 0
	last := sorted[0].Value
	for i, ts := range timestamps {
		for idx < len(sorted) && !sorted[idx].Time.After(ts) {
			last = sorted[idx].Value
			idx++
		}
		values[i] = last
	}
	return values
}

func AverageSeries(series [][]float64) []float64 {
	if len(series) == 0 {
		return nil
	}
	n := len(series[0])
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		sum := 0.0
		count := 0
		for _, s := range series {
			if i < len(s) && !math.IsNaN(s[i]) {
				sum += s[i]
				count++
			}
		}
		if count == 0 {
			out[i] = math.NaN()
		} else {
			out[i] = sum / float64(count)
		}
	}
	return out
}
