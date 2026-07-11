package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
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

func sparseAxisLabels(timestamps []time.Time) []string {
	labels := make([]string, len(timestamps))
	if len(timestamps) == 0 {
		return labels
	}
	last := len(timestamps) - 1
	labels[0] = timestamps[0].Format("15:04")
	labels[last] = timestamps[last].Format("15:04")
	if last > 1 {
		labels[last/2] = timestamps[last/2].Format("15:04")
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
// background tint and glow. Temperature (HasTemperature/TempValue/ChartSVG)
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

// Nominal internal SVG coordinate-space heights for the temperature chart.
// These are unrelated to temperature.chart_height (which now sizes the
// room *card*, not the chart) — preserveAspectRatio="none" stretches the
// chart to fill whatever height its flex-grown .ha-room-chart box ends up
// with, so this only needs to give the chart's internal margin/plot-area
// proportions a sensible shape, not match any real pixel measurement.
const sparklineNominalHeight = 60
const barChartNominalHeight = 90

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

	rangeDur, _ := time.ParseDuration(a.cfg.Temperature.Range)
	pollInterval, _ := time.ParseDuration(a.cfg.Live.PollInterval)
	now := time.Now()
	timestamps := hass.BuildTimestamps(now, rangeDur, a.cfg.Temperature.MaxPoints)
	axisLabels := sparseAxisLabels(timestamps)

	var allTempIDs []string
	for _, card := range cards {
		if card.Temperature != nil {
			allTempIDs = append(allTempIDs, card.Temperature.EntityIDs...)
		}
	}

	history, err := a.client.FetchHistory(ctx, allTempIDs, now.Add(-rangeDur), now)
	if err != nil {
		log.Printf("fetch history: %v", err)
		history = map[string][]hass.HistoryPoint{}
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
				series = append(series, hass.StepForwardFill(points, timestamps))
			}
			avg := hass.AverageSeries(series)
			if len(avg) == 0 || math.IsNaN(avg[len(avg)-1]) {
				view.TempNoData = true
			} else {
				view.TempValue = fmt.Sprintf("%.1f°", avg[len(avg)-1])
				if a.cfg.Temperature.ChartStyle == "bars" {
					barOpts := render.BarChartOptions{Width: 220, Height: barChartNominalHeight, ClassName: "ha-room-chart"}
					view.ChartSVG = render.BarChart(avg, axisLabels, view.TempValue, barOpts)
				} else {
					view.ChartSVG = render.Sparkline(avg, axisLabels, render.SparklineOptions{Width: 220, Height: sparklineNominalHeight, ClassName: "ha-room-chart"})
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
