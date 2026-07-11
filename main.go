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

func (a *app) buildModel(ctx context.Context) (hass.Model, error) {
	rooms, err := a.cache.Get(ctx)
	if err != nil {
		return hass.Model{}, fmt.Errorf("fetch areas: %w", err)
	}
	states, err := a.client.FetchStates(ctx)
	if err != nil {
		return hass.Model{}, fmt.Errorf("fetch states: %w", err)
	}
	return hass.BuildModel(rooms, states, hass.ClassificationConfig{
		ContactDeviceClasses: a.cfg.Sensors.ContactDeviceClasses,
		MotionDeviceClasses:  a.cfg.Sensors.MotionDeviceClasses,
	}), nil
}

func (a *app) widgetHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	w.Header().Set("Widget-Title", a.cfg.Title)
	w.Header().Set("Widget-Content-Type", "html")

	model, err := a.buildModel(ctx)
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
	for _, tr := range model.TemperatureRooms {
		allTempIDs = append(allTempIDs, tr.EntityIDs...)
	}

	history, err := a.client.FetchHistory(ctx, allTempIDs, now.Add(-rangeDur), now)
	if err != nil {
		log.Printf("fetch history: %v", err)
		history = map[string][]hass.HistoryPoint{}
	}

	tempViews := make([]render.TemperatureRoomView, len(model.TemperatureRooms))
	for i, tr := range model.TemperatureRooms {
		var series [][]float64
		for _, id := range tr.EntityIDs {
			points, ok := history[id]
			if !ok || len(points) == 0 {
				continue
			}
			series = append(series, hass.StepForwardFill(points, timestamps))
		}
		avg := hass.AverageSeries(series)
		if len(avg) == 0 || math.IsNaN(avg[len(avg)-1]) {
			tempViews[i] = render.TemperatureRoomView{Room: tr.Room, NoData: true}
			continue
		}

		value := fmt.Sprintf("%.1f°", avg[len(avg)-1])
		var svg string
		if a.cfg.Temperature.ChartStyle == "bars" {
			barOpts := render.BarChartOptions{Width: 220, Height: float64(a.cfg.Temperature.ChartHeight + 27)}
			svg = render.BarChart(avg, axisLabels, value, barOpts)
		} else {
			svg = render.Sparkline(avg, axisLabels, render.SparklineOptions{Width: 220, Height: float64(a.cfg.Temperature.ChartHeight + 12)})
		}
		tempViews[i] = render.TemperatureRoomView{Room: tr.Room, Value: value, SVG: svg}
	}

	lightViews := make([]render.LightRoomView, len(model.LightRooms))
	for i, lr := range model.LightRooms {
		lightViews[i] = render.LightRoomView{Room: lr.Room, On: lr.On, Total: lr.Total}
	}
	sensorViews := make([]render.SensorView, len(model.Sensors))
	for i, s := range model.Sensors {
		sensorViews[i] = render.SensorView{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}

	widgetData := render.WidgetData{
		ChartStyle:       a.cfg.Temperature.ChartStyle,
		TemperatureRooms: tempViews,
		LightRooms:       lightViews,
		Sensors:          sensorViews,
		LiveURL:          liveURL(a.cfg.PublicURL),
		PollIntervalMS:   int(pollInterval.Milliseconds()),
		PauseWhenHidden:  a.cfg.Live.PauseWhenHidden != nil && *a.cfg.Live.PauseWhenHidden,
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, render.RenderWidget(widgetData))
}

func (a *app) liveHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	w.Header().Set("Access-Control-Allow-Origin", "*")

	model, err := a.buildModel(ctx)
	if err != nil {
		log.Printf("home assistant unavailable: %v", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	lightViews := make([]render.LightRoomView, len(model.LightRooms))
	for i, lr := range model.LightRooms {
		lightViews[i] = render.LightRoomView{Room: lr.Room, On: lr.On, Total: lr.Total}
	}
	sensorViews := make([]render.SensorView, len(model.Sensors))
	for i, s := range model.Sensors {
		sensorViews[i] = render.SensorView{Name: s.Name, Attention: s.Attention, Label: s.Label}
	}

	body, err := render.RenderLive(lightViews, sensorViews)
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
