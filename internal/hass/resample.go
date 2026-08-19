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

// BuildDayTimestamps builds an evenly spaced timestamp series spanning the
// LOCAL calendar day (midnight to the following midnight, in now's own
// location) that now falls on — a fixed window, unlike BuildTimestamps'
// rolling one, so a daytime/nighttime highlight computed against it lands
// wherever sunrise/sunset actually fall rather than always at the trailing
// edge (mirrors how Glance's own built-in Weather widget can center its
// daylight band: a full calendar day naturally bookends night at both
// edges with day in the middle, which a "last 24h ending now" window
// cannot generally do). currentIndex is the index of the latest timestamp
// that is not after now — later indices represent hours later today that
// haven't happened yet and have no real data (callers should leave those
// as gaps, not fabricate values). If maxPoints < 2, returns a single
// timestamp at today's midnight with currentIndex 0.
func BuildDayTimestamps(now time.Time, maxPoints int) (timestamps []time.Time, currentIndex int) {
	loc := now.Location()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if maxPoints < 2 {
		return []time.Time{dayStart}, 0
	}
	dayEnd := dayStart.Add(24 * time.Hour)
	step := 24 * time.Hour / time.Duration(maxPoints-1)
	timestamps = make([]time.Time, maxPoints)
	for i := 0; i < maxPoints; i++ {
		timestamps[i] = dayStart.Add(step * time.Duration(i))
	}
	timestamps[maxPoints-1] = dayEnd

	currentIndex = 0
	for i, ts := range timestamps {
		if ts.After(now) {
			break
		}
		currentIndex = i
	}
	return timestamps, currentIndex
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

// BuildCenteredDayBuckets lays out `buckets` equal-length buckets across a
// 24-hour window centred on `center` (in practice solar noon — see
// SunState.SolarNoon), and returns one sample timestamp per bucket, the
// index of the bucket containing now, and the window's start.
//
// This replaces BuildDayTimestamps for the bars chart style. A fixed local
// calendar day only *happens* to put the daylight band near the middle,
// and only at latitudes and seasons where sunrise and sunset sit roughly
// symmetrically around midday — in Ireland in December, or anywhere far
// north in summer, it drifts noticeably off-centre, and the band's
// position wanders through the year for no reason a reader can see.
// Anchoring the window on solar noon instead puts the band dead centre by
// construction, every day of the year, and lets the time axis fall out of
// the window rather than the other way round.
//
// Worth being explicit about, since this widget otherwise mirrors it:
// Glance's own Weather widget does NOT do this. It hardcodes twelve
// two-hour buckets across the local calendar day and twelve matching label
// strings (`timeLabels12h` in widget-weather.go), and its band lands
// wherever sunrise/sunset happen to fall. This is a deliberate departure.
//
// Three details:
//
//   - The centre is snapped to the nearest bucket boundary counted from
//     local midnight before the window is derived from it, so every bucket
//     edge lands on a whole clock hour (an even one, for the usual twelve
//     buckets). Without that, a 13:37 solar noon would produce an axis
//     reading 3:37am / 5:37am / … instead of 4am / 6am / ….
//
//   - The window is then slid by whole days until it contains now. Solar
//     noon is a property of a date, and the date whose noon-centred window
//     covers "now" is not always today's: at 23:00 the window centred on
//     tomorrow's noon has not started yet, and the right one is the one
//     centred on today's.
//
//   - Each bucket's sample timestamp is its END, matching how Weather
//     labels its columns (its bucket for hours 0–1 is labelled "2am"). The
//     bucket containing now therefore samples slightly in the future, which
//     is exactly what makes it show the newest known reading once
//     StepForwardFill carries the last value forward.
func BuildCenteredDayBuckets(now, center time.Time, buckets int) (timestamps []time.Time, currentIndex int, windowStart time.Time) {
	if buckets < 1 {
		buckets = 1
	}
	loc := now.Location()
	center = center.In(loc)
	bucketLen := 24 * time.Hour / time.Duration(buckets)

	midnight := time.Date(center.Year(), center.Month(), center.Day(), 0, 0, 0, 0, loc)
	steps := math.Round(float64(center.Sub(midnight)) / float64(bucketLen))
	windowStart = midnight.Add(time.Duration(steps) * bucketLen).Add(-12 * time.Hour)

	days := math.Floor(float64(now.Sub(windowStart)) / float64(24*time.Hour))
	windowStart = windowStart.Add(time.Duration(days) * 24 * time.Hour)

	timestamps = make([]time.Time, buckets)
	for i := range timestamps {
		timestamps[i] = windowStart.Add(bucketLen * time.Duration(i+1))
	}

	currentIndex = int(now.Sub(windowStart) / bucketLen)
	if currentIndex < 0 {
		currentIndex = 0
	}
	if currentIndex > buckets-1 {
		currentIndex = buckets - 1
	}
	return timestamps, currentIndex, windowStart
}

// StepForwardFillStrict is StepForwardFill without its "fall back to the
// first known point" behaviour: a timestamp earlier than anything in
// `points` comes back NaN instead of borrowing the earliest reading.
//
// The difference matters for the bars chart's projected columns, which
// sample 24 hours behind each not-yet-happened bucket to stand in for the
// reading that hasn't been taken yet (see projectYesterday in main.go).
// StepForwardFill's fallback is right for the leading edge of a live
// window — an entity whose first ever state landed a few minutes into it
// should not leave a hole — but wrong here: for a sensor that has only
// existed for an hour it would paint a full, flat, confident-looking row of
// "yesterday" out of a single reading. There simply is no yesterday for
// those buckets, and NaN says so.
func StepForwardFillStrict(points []HistoryPoint, timestamps []time.Time) []float64 {
	values := StepForwardFill(points, timestamps)
	if len(points) == 0 {
		return values
	}
	earliest := points[0].Time
	for _, p := range points[1:] {
		if p.Time.Before(earliest) {
			earliest = p.Time
		}
	}
	for i, ts := range timestamps {
		if ts.Before(earliest) {
			values[i] = math.NaN()
		}
	}
	return values
}
