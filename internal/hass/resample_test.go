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

func TestBuildDayTimestamps_SpansLocalMidnightToMidnight(t *testing.T) {
	now := time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC)
	timestamps, _ := BuildDayTimestamps(now, 5)

	if len(timestamps) != 5 {
		t.Fatalf("len(timestamps) = %d, want 5", len(timestamps))
	}
	wantStart := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 7, 11, 0, 0, 0, 0, time.UTC)
	if !timestamps[0].Equal(wantStart) {
		t.Errorf("timestamps[0] = %v, want %v (today's local midnight)", timestamps[0], wantStart)
	}
	if !timestamps[4].Equal(wantEnd) {
		t.Errorf("timestamps[4] = %v, want %v (next local midnight)", timestamps[4], wantEnd)
	}
}

func TestBuildDayTimestamps_CurrentIndexIsLatestNotAfterNow(t *testing.T) {
	// Day: 00:00, 06:00, 12:00, 18:00, 24:00. now=15:30 falls between
	// index 2 (12:00) and index 3 (18:00) — current must be index 2.
	now := time.Date(2026, 7, 10, 15, 30, 0, 0, time.UTC)
	_, currentIndex := BuildDayTimestamps(now, 5)
	if currentIndex != 2 {
		t.Errorf("currentIndex = %d, want 2", currentIndex)
	}
}

func TestBuildDayTimestamps_NowExactlyAtMidnightIsIndexZero(t *testing.T) {
	now := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)
	_, currentIndex := BuildDayTimestamps(now, 5)
	if currentIndex != 0 {
		t.Errorf("currentIndex = %d, want 0", currentIndex)
	}
}

func TestBuildDayTimestamps_NowLateInDayIsSecondToLastIndex(t *testing.T) {
	// Day: 00:00, 06:00, 12:00, 18:00, 24:00(next day). now=23:59 is after
	// 18:00 but still before the next day's midnight bucket — index 3.
	now := time.Date(2026, 7, 10, 23, 59, 0, 0, time.UTC)
	_, currentIndex := BuildDayTimestamps(now, 5)
	if currentIndex != 3 {
		t.Errorf("currentIndex = %d, want 3", currentIndex)
	}
}

// --- Solar-noon-centred window ---

func TestSunState_SolarNoon_AboveHorizonUsesThePreviousRising(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	// It is daytime: the sun set-time still ahead of us belongs to the day
	// that started at a rising already in the past, and next_rising is
	// tomorrow's. Solar noon must come out as today's midday, not the
	// midpoint of the coming NIGHT.
	s := SunState{
		State:       "above_horizon",
		NextSetting: time.Date(2026, 8, 19, 21, 0, 0, 0, loc),
		NextRising:  time.Date(2026, 8, 20, 6, 20, 0, 0, loc),
	}
	noon, ok := s.SolarNoon()
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := time.Date(2026, 8, 19, 13, 40, 0, 0, loc)
	if !noon.Equal(want) {
		t.Errorf("noon = %s, want %s", noon.In(loc), want)
	}
}

func TestSunState_SolarNoon_BelowHorizonUsesTheComingDay(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	// Night, either side of midnight: next_rising already precedes
	// next_setting, so the pair brackets one day as-is.
	s := SunState{
		State:       "below_horizon",
		NextRising:  time.Date(2026, 8, 20, 6, 20, 0, 0, loc),
		NextSetting: time.Date(2026, 8, 20, 21, 0, 0, 0, loc),
	}
	noon, ok := s.SolarNoon()
	if !ok {
		t.Fatal("ok = false, want true")
	}
	want := time.Date(2026, 8, 20, 13, 40, 0, 0, loc)
	if !noon.Equal(want) {
		t.Errorf("noon = %s, want %s", noon.In(loc), want)
	}
}

func TestSunState_SolarNoon_MissingAttributesReportsNotOK(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	cases := map[string]SunState{
		"both missing": {State: "above_horizon"},
		"no rising":    {State: "above_horizon", NextSetting: time.Date(2026, 8, 19, 21, 0, 0, 0, loc)},
		"no setting":   {State: "above_horizon", NextRising: time.Date(2026, 8, 20, 6, 20, 0, 0, loc)},
	}
	for name, s := range cases {
		if _, ok := s.SolarNoon(); ok {
			t.Errorf("%s: ok = true, want false", name)
		}
	}
}

// TestBuildCenteredDayBuckets_SnapsWindowToWholeBucketBoundaries pins the
// reason the axis reads 4am/6am/… and not 3:40am/5:40am/…: solar noon is a
// ragged time, so it is rounded to the nearest bucket boundary counted from
// local midnight before the window is derived from it.
func TestBuildCenteredDayBuckets_SnapsWindowToWholeBucketBoundaries(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	noon := time.Date(2026, 8, 19, 13, 40, 0, 0, loc)
	now := time.Date(2026, 8, 19, 20, 30, 0, 0, loc)

	timestamps, _, windowStart := BuildCenteredDayBuckets(now, noon, 12)

	// 13:40 rounds to the 14:00 boundary; the window is that ± 12h.
	wantStart := time.Date(2026, 8, 19, 2, 0, 0, 0, loc)
	if !windowStart.Equal(wantStart) {
		t.Errorf("windowStart = %s, want %s", windowStart.In(loc), wantStart)
	}
	if len(timestamps) != 12 {
		t.Fatalf("len(timestamps) = %d, want 12", len(timestamps))
	}
	// Bucket ENDS, every 2h, first one 2h after the window opens.
	for i, ts := range timestamps {
		want := wantStart.Add(time.Duration(i+1) * 2 * time.Hour)
		if !ts.Equal(want) {
			t.Errorf("timestamps[%d] = %s, want %s", i, ts.In(loc), want)
		}
		if h := ts.In(loc).Hour(); h%2 != 0 || ts.In(loc).Minute() != 0 {
			t.Errorf("timestamps[%d] = %s, want a whole even hour", i, ts.In(loc))
		}
	}
	if last := timestamps[11]; !last.Equal(wantStart.Add(24 * time.Hour)) {
		t.Errorf("last timestamp = %s, want the window to close exactly 24h after it opened", last.In(loc))
	}
}

// TestBuildCenteredDayBuckets_WindowAlwaysContainsNow covers the reason the
// window is slid by whole days: solar noon belongs to a date, and the date
// whose noon-centred window covers "now" is not always the one the sun
// state describes. Late in the evening the window centred on the COMING
// day's noon has not started yet, and the correct one is the previous day's.
func TestBuildCenteredDayBuckets_WindowAlwaysContainsNow(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	noon := time.Date(2026, 8, 20, 13, 40, 0, 0, loc) // the COMING day's noon

	for _, now := range []time.Time{
		time.Date(2026, 8, 19, 23, 30, 0, 0, loc), // late evening, before that window opens
		time.Date(2026, 8, 20, 0, 30, 0, 0, loc),  // just after midnight
		time.Date(2026, 8, 20, 2, 0, 0, 0, loc),   // exactly on a window boundary
		time.Date(2026, 8, 20, 13, 40, 0, 0, loc), // solar noon itself
		time.Date(2026, 8, 21, 1, 59, 0, 0, loc),  // last minute of that window
	} {
		timestamps, currentIdx, windowStart := BuildCenteredDayBuckets(now, noon, 12)
		windowEnd := windowStart.Add(24 * time.Hour)
		if now.Before(windowStart) || !now.Before(windowEnd) {
			t.Errorf("now = %s falls outside window [%s, %s)", now, windowStart.In(loc), windowEnd.In(loc))
			continue
		}
		if currentIdx < 0 || currentIdx > 11 {
			t.Errorf("now = %s: currentIdx = %d, out of range", now, currentIdx)
			continue
		}
		// currentIdx must name the bucket now actually falls in: its end is
		// the first timestamp after now.
		if !timestamps[currentIdx].After(now) {
			t.Errorf("now = %s: bucket %d ends at %s, which is not after now", now, currentIdx, timestamps[currentIdx].In(loc))
		}
		if currentIdx > 0 && timestamps[currentIdx-1].After(now) {
			t.Errorf("now = %s: bucket %d is not the first one ending after now", now, currentIdx)
		}
	}
}

// TestBuildCenteredDayBuckets_DaylightLandsInTheMiddle is the point of the
// whole exercise: whatever the season, the buckets that fall between
// sunrise and sunset must sit symmetrically about the chart's centre, so
// the daylight band never drifts to one side. A calendar-day window cannot
// promise this — see the doc comment on BuildCenteredDayBuckets.
func TestBuildCenteredDayBuckets_DaylightLandsInTheMiddle(t *testing.T) {
	loc := time.FixedZone("IST", 3600)

	cases := []struct {
		name           string
		sunrise, sunse time.Time
	}{
		{
			name:    "long summer day",
			sunrise: time.Date(2026, 6, 21, 5, 0, 0, 0, loc),
			sunse:   time.Date(2026, 6, 21, 22, 20, 0, 0, loc),
		},
		{
			name:    "short winter day",
			sunrise: time.Date(2026, 12, 21, 8, 40, 0, 0, loc),
			sunse:   time.Date(2026, 12, 21, 16, 20, 0, 0, loc),
		},
	}
	for _, tc := range cases {
		noon := tc.sunrise.Add(tc.sunse.Sub(tc.sunrise) / 2)
		now := tc.sunrise.Add(2 * time.Hour)
		timestamps, _, _ := BuildCenteredDayBuckets(now, noon, 12)

		first, last := -1, -1
		for i, ts := range timestamps {
			if !ts.Before(tc.sunrise) && !ts.After(tc.sunse) {
				if first == -1 {
					first = i
				}
				last = i
			}
		}
		if first == -1 {
			t.Errorf("%s: no bucket fell between sunrise and sunset", tc.name)
			continue
		}
		// Gap of empty buckets on each side must match within one bucket —
		// the snapping to whole hours can only ever cost half a bucket a side.
		leftGap, rightGap := first, len(timestamps)-1-last
		if diff := leftGap - rightGap; diff < -1 || diff > 1 {
			t.Errorf("%s: daylight spans buckets [%d,%d] of 0..11 — %d empty on the left vs %d on the right, want them within one bucket of each other",
				tc.name, first, last, leftGap, rightGap)
		}
	}
}

func TestBuildCenteredDayBuckets_SingleBucketDegeneratesCleanly(t *testing.T) {
	loc := time.FixedZone("IST", 3600)
	noon := time.Date(2026, 8, 19, 13, 40, 0, 0, loc)
	now := time.Date(2026, 8, 19, 20, 30, 0, 0, loc)
	for _, buckets := range []int{0, 1} {
		timestamps, currentIdx, windowStart := BuildCenteredDayBuckets(now, noon, buckets)
		if len(timestamps) != 1 || currentIdx != 0 {
			t.Errorf("buckets=%d: got %d timestamps / currentIdx %d, want 1 / 0", buckets, len(timestamps), currentIdx)
		}
		if now.Before(windowStart) || !now.Before(windowStart.Add(24*time.Hour)) {
			t.Errorf("buckets=%d: now outside the window", buckets)
		}
	}
}

// --- Strict fill (projected columns) ---

func TestStepForwardFillStrict_LeavesNaNBeforeTheFirstKnownPoint(t *testing.T) {
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	points := []HistoryPoint{
		{Time: base.Add(2 * time.Hour), Value: 21},
		{Time: base.Add(4 * time.Hour), Value: 23},
	}
	timestamps := []time.Time{base, base.Add(time.Hour), base.Add(3 * time.Hour), base.Add(5 * time.Hour)}

	strict := StepForwardFillStrict(points, timestamps)
	if !math.IsNaN(strict[0]) || !math.IsNaN(strict[1]) {
		t.Errorf("strict[0..1] = %v, %v, want NaN for timestamps earlier than any reading", strict[0], strict[1])
	}
	if strict[2] != 21 || strict[3] != 23 {
		t.Errorf("strict[2..3] = %v, %v, want 21, 23", strict[2], strict[3])
	}

	// The lenient version is what the leading edge of a live window wants,
	// and is deliberately different here.
	lenient := StepForwardFill(points, timestamps)
	if lenient[0] != 21 {
		t.Errorf("lenient[0] = %v, want the first known point carried backwards (21)", lenient[0])
	}
}

func TestStepForwardFillStrict_UnsortedPointsStillFindTheEarliest(t *testing.T) {
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	points := []HistoryPoint{
		{Time: base.Add(4 * time.Hour), Value: 23},
		{Time: base.Add(1 * time.Hour), Value: 21},
	}
	strict := StepForwardFillStrict(points, []time.Time{base, base.Add(2 * time.Hour)})
	if !math.IsNaN(strict[0]) {
		t.Errorf("strict[0] = %v, want NaN", strict[0])
	}
	if strict[1] != 21 {
		t.Errorf("strict[1] = %v, want 21", strict[1])
	}
}

func TestStepForwardFillStrict_NoPointsIsAllNaN(t *testing.T) {
	base := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	strict := StepForwardFillStrict(nil, []time.Time{base, base.Add(time.Hour)})
	for i, v := range strict {
		if !math.IsNaN(v) {
			t.Errorf("strict[%d] = %v, want NaN", i, v)
		}
	}
}
