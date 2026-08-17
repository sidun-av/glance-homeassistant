package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/bits"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/sidun-av/glance-homeassistant/internal/hass"
	"github.com/sidun-av/glance-homeassistant/internal/render"
)

// app bundles the long-lived dependencies each handler needs.
type app struct {
	cfg    *Config
	cache  *hass.AreaCache
	client *hass.Client
}

func newApp(cfg *Config) *app {
	client := hass.New(cfg.HomeAssistant.URL, cfg.HomeAssistant.Token)
	return &app{cfg: cfg, cache: hass.NewAreaCache(client, 5*time.Minute), client: client}
}

func liveURL(publicURL string) string {
	return strings.TrimRight(publicURL, "/") + "/live.json"
}

// axisLabelIntervals is the number of dyadic subdivisions used to pick
// sparse timeline label candidates (see sparseAxisLabels): 8 intervals
// across 9 evenly spaced candidate positions. Must stay a power of two for
// axisLabelTier's bit trick to produce a clean tier assignment.
const axisLabelIntervals = 8

// axisLabelTier returns the dyadic subdivision level at which candidate k
// (of axisLabelIntervals+1 evenly spaced candidates, k in
// [0,axisLabelIntervals]) first appears: 0 for the two endpoints, 1 for
// the midpoint, 2 for the quarter points, and so on. Every tier's
// positions are a superset of all lower tiers', so revealing a higher
// tier (see the ha-chart-axis @container rules in
// internal/render/template.go) only adds labels — it never repositions
// ones already shown, which matters for a smooth reveal as a room's card
// grows wider.
func axisLabelTier(k, total int) int {
	if k == 0 || k == total {
		return 0
	}
	return bits.TrailingZeros(uint(total)) - bits.TrailingZeros(uint(k))
}

// sparseAxisLabels picks a small set of evenly spaced, tiered time labels
// for a room's temperature chart x-axis (see render.AxisLabelsRow), rather
// than a fixed first/middle/last regardless of how much room the chart
// actually has. It samples axisLabelIntervals+1 candidate positions across
// the full timestamp range and keeps one label per distinct timestamp
// index, tagged with the tier at which it should start appearing — CSS
// reveals more of them as the room's card gets wider (see styleBlock),
// without any of the already-shown labels moving.
func sparseAxisLabels(timestamps []time.Time) []render.AxisLabel {
	n := len(timestamps)
	if n == 0 {
		return nil
	}

	bestTier := make(map[int]int, axisLabelIntervals+1)
	for k := 0; k <= axisLabelIntervals; k++ {
		idx := int(math.Round(float64(k) * float64(n-1) / float64(axisLabelIntervals)))
		tier := axisLabelTier(k, axisLabelIntervals)
		if existing, ok := bestTier[idx]; !ok || tier < existing {
			bestTier[idx] = tier
		}
	}

	indices := make([]int, 0, len(bestTier))
	for idx := range bestTier {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	labels := make([]render.AxisLabel, len(indices))
	for i, idx := range indices {
		labels[i] = render.AxisLabel{Text: timestamps[idx].Format("3pm"), Tier: bestTier[idx]}
	}
	return labels
}

// barColumnTimeLabels picks the sparse per-column time labels for the
// "bars" chart style: every 4th of the (fixed, barColumnsCount-long)
// timestamps slice, mirroring Weather's own hardcoded nth-child(3,7,11)
// sparse pattern for its 12 columns — a fixed small column count doesn't
// need the denser sparkline's dyadic-tier progressive-reveal system, a
// flat "every Nth" is exactly what the reference implementation does.
func barColumnTimeLabels(timestamps []time.Time) []string {
	labels := make([]string, len(timestamps))
	for i := range timestamps {
		if i%4 == 0 {
			labels[i] = timestamps[i].Format("3pm")
		}
	}
	return labels
}

// sizeClassForWeight maps a RoomCard's Weight onto the CSS class driving
// its card's flex-grow/min-height tier (see internal/render/template.go's
// styleBlock). Thresholds are fixed, not configurable — see the design
// spec's "Card sizing" section for the rationale.
func sizeClassForWeight(weight int) string {
	switch {
	case weight >= 5:
		return "ha-size-lg"
	case weight >= 3:
		return "ha-size-md"
	default:
		return ""
	}
}

// roomCardView maps a classified hass.RoomCard onto the render package's
// view type, computing the derived Lit/Occupied flags used for the card's
// background tint and glow. Temperature (HasTemperature/TempValue/ChartHTML)
// is populated separately by widgetHandler, since only it fetches history
// — liveHandler never needs a chart.
func roomCardView(card hass.RoomCard) render.RoomCardView {
	view := render.RoomCardView{
		Room:      card.Room,
		SizeClass: sizeClassForWeight(card.Weight),
	}
	for _, l := range card.Lights {
		view.Lights = append(view.Lights, render.LightView{
			EntityID: l.EntityID,
			IconSVG:  render.LightIcon(l.Icon),
			On:       l.On,
		})
		if l.On {
			view.Lit = true
		}
	}
	for _, o := range card.Occupancy {
		view.Occupancy = append(view.Occupancy, render.SensorBadgeView{Name: o.Name, Attention: o.Attention, Label: o.Label})
		if o.Attention {
			view.Occupied = true
		}
	}
	for _, c := range card.Contacts {
		view.Contacts = append(view.Contacts, render.SensorBadgeView{Name: c.Name, Attention: c.Attention, Label: c.Label})
	}
	return view
}

func (a *app) buildModel(ctx context.Context) ([]hass.RoomCard, error) {
	rooms, err := a.cache.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch areas: %w", err)
	}
	states, err := a.client.FetchStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch states: %w", err)
	}
	return hass.BuildModel(rooms, states, hass.ClassificationConfig{
		ContactDeviceClasses: a.cfg.Sensors.ContactDeviceClasses,
		MotionDeviceClasses:  a.cfg.Sensors.MotionDeviceClasses,
	}), nil
}

// Nominal internal SVG coordinate-space height for the sparkline chart
// style. Unrelated to temperature.chart_height (which sizes the room
// *card*, not the chart) — preserveAspectRatio="none" stretches the
// sparkline to fill whatever height its flex-grown box ends up with, so
// this only needs to give its internal margin/plot-area proportions a
// sensible shape, not match any real pixel measurement. The "bars" chart
// style no longer uses SVG at all (see internal/render/barchart.go's
// BarColumns) so it has no equivalent constant.
const sparklineNominalHeight = 60

// barColumnsCount is how many two-hour buckets the "bars" chart style
// shows across a full calendar day — fixed at 12, mirroring Glance's own
// built-in Weather widget exactly (its bars are also always 12 columns
// over 24h). This intentionally does NOT scale with
// temperature.max_points the way the (denser) sparkline style still does:
// a dense sparkline reads fine as a smooth line, but the same density as
// discrete bars looked visibly noisy/cramped, especially on narrow
// (mobile) widths — Weather's chunky 12-column layout is the reference
// this widget is matching, not a data-resolution choice.
const barColumnsCount = 12

func (a *app) widgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Widget-Title", a.cfg.Title)
	w.Header().Set("Widget-Content-Type", "html")

	cards, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, render.RenderUnavailable())
		return
	}

	pollInterval, _ := time.ParseDuration(a.cfg.Live.PollInterval)
	now := time.Now()
	// A fixed local-calendar-day window (not the old "last N hours ending
	// now" rolling one) — see BuildDayTimestamps' doc comment for why:
	// a rolling window can't generally center a daytime band, a fixed
	// full-day one naturally does. Temperature.Range no longer drives the
	// window's length (it's always a full day); it's effectively
	// vestigial now and a candidate for removal in a follow-up.
	//
	// Point count depends on chart_style: "bars" always uses a fixed 12
	// (see barColumnsCount's doc comment — matching Weather's own column
	// count, not a data-resolution choice), sparkline keeps the
	// configurable, denser temperature.max_points.
	maxPoints := a.cfg.Temperature.MaxPoints
	if a.cfg.Temperature.ChartStyle == "bars" {
		maxPoints = barColumnsCount
	}
	timestamps, currentIdx := hass.BuildDayTimestamps(now, maxPoints)
	dayStart := timestamps[0]
	axisLabels := sparseAxisLabels(timestamps)

	// nanOutFuture blanks every index after currentIdx — those timestamps
	// are later today and haven't happened yet, so StepForwardFill's
	// carry-forward behavior would otherwise repeat the latest known
	// value into them instead of correctly leaving a gap.
	nanOutFuture := func(series []float64) {
		for i := currentIdx + 1; i < len(series); i++ {
			series[i] = math.NaN()
		}
	}

	var allTempIDs []string
	for _, card := range cards {
		if card.Temperature != nil {
			allTempIDs = append(allTempIDs, card.Temperature.EntityIDs...)
		}
	}

	history, err := a.client.FetchHistory(ctx, allTempIDs, dayStart, now)
	if err != nil {
		log.Printf("fetch history: %v", err)
		history = map[string][]hass.HistoryPoint{}
	}

	// Fetched once, not per-room — sun.sun is a single global entity, not
	// tied to any particular room. A fetch failure degrades gracefully:
	// StepForwardFill on an empty slice returns all-NaN, and NaN >= 0.5 is
	// false in Go, so isDaytime ends up all-false and BarChart simply
	// omits the daytime band rather than the whole widget failing.
	sunPoints, err := a.client.FetchSunHistory(ctx, dayStart, now)
	if err != nil {
		log.Printf("fetch sun history: %v", err)
		sunPoints = nil
	}
	sunSeries := hass.StepForwardFill(sunPoints, timestamps)
	nanOutFuture(sunSeries)
	isDaytime := make([]bool, len(sunSeries))
	for i, v := range sunSeries {
		isDaytime[i] = v >= 0.5
	}

	views := make([]render.RoomCardView, len(cards))
	for i, card := range cards {
		view := roomCardView(card)

		if card.Temperature != nil {
			view.HasTemperature = true

			var series [][]float64
			for _, id := range card.Temperature.EntityIDs {
				points, ok := history[id]
				if !ok || len(points) == 0 {
					continue
				}
				filled := hass.StepForwardFill(points, timestamps)
				nanOutFuture(filled)
				series = append(series, filled)
			}
			avg := hass.AverageSeries(series)
			if len(avg) == 0 || currentIdx >= len(avg) || math.IsNaN(avg[currentIdx]) {
				view.TempNoData = true
			} else {
				view.TempValue = fmt.Sprintf("%.1f°", avg[currentIdx])
				if a.cfg.Temperature.ChartStyle == "bars" {
					barData := render.BarChartData{Values: avg, IsDaytime: isDaytime, CurrentLabel: view.TempValue, CurrentIndex: currentIdx, TimeLabels: barColumnTimeLabels(timestamps)}
					view.ChartHTML = render.BarColumns(barData, "ha-bar-cols")
					view.AxisRowHTML = "" // bars renders its own per-column time labels, no separate axis row
				} else {
					// Sparkline doesn't handle NaN (unlike BarColumns, which
					// was updated to skip it) — trim the not-yet-happened
					// tail rather than feed it invalid points. Known gap:
					// this loses the "blank space for later today" look
					// bars gets; acceptable since sparkline isn't the
					// default chart_style.
					view.ChartHTML = render.Sparkline(avg[:currentIdx+1], render.SparklineOptions{Width: 220, Height: sparklineNominalHeight, ClassName: "ha-room-chart"})
					view.AxisRowHTML = render.AxisLabelsRow(axisLabels)
				}
			}
		}

		views[i] = view
	}

	widgetData := render.WidgetData{
		Rooms:           views,
		CardMinHeight:   a.cfg.Temperature.ChartHeight,
		LiveURL:         liveURL(a.cfg.PublicURL),
		PollIntervalMS:  int(pollInterval.Milliseconds()),
		PauseWhenHidden: a.cfg.Live.PauseWhenHidden != nil && *a.cfg.Live.PauseWhenHidden,
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, render.RenderWidget(widgetData))
}

func (a *app) liveHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	w.Header().Set("Access-Control-Allow-Origin", "*")

	cards, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	views := make([]render.RoomCardView, len(cards))
	for i, card := range cards {
		views[i] = roomCardView(card)
	}

	body, err := render.RenderLive(views)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func newMux(cfg *Config, a *app) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/widget", a.widgetHandler)
	mux.HandleFunc("/live.json", a.liveHandler)

	// A reverse proxy in front of this service (see README's "Expose this
	// service to your browser" step) may forward a Custom Location's full
	// original path instead of stripping the public_url prefix — that
	// depends on details like a trailing slash on its own proxy_pass, which
	// not every proxy UI (e.g. Nginx Proxy Manager's basic Custom Location
	// form) makes easy to get right. Registering the same handlers under
	// that prefix too means live updates work either way, without the
	// user needing to fight their reverse proxy's path-stripping behavior.
	// A full origin (e.g. a dedicated LAN port, not a path) is a distinct
	// listener reached directly with no such prefix ever attached, so this
	// only applies when public_url is itself a path.
	if prefix := strings.TrimRight(cfg.PublicURL, "/"); strings.HasPrefix(prefix, "/") {
		mux.HandleFunc(prefix+"/live.json", a.liveHandler)
	}
	return mux
}

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/config.yml"
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	a := newApp(cfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, newMux(cfg, a)))
}
