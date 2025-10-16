package telemetry

import (
	"time"

	"authway/src/server/internal/config"
	"github.com/microsoft/ApplicationInsights-Go/appinsights"
	"go.uber.org/zap"
)

// Client wraps Application Insights telemetry client (optional)
type Client struct {
	client  appinsights.TelemetryClient
	enabled bool
	logger  *zap.Logger
}

// NewClient creates a new Application Insights client
// Returns a disabled client if connection string is not configured
func NewClient(cfg *config.ApplicationInsightsConfig, logger *zap.Logger) *Client {
	if cfg == nil || !cfg.Enabled || cfg.ConnectionString == "" {
		logger.Info("Application Insights is disabled or not configured")
		return &Client{
			enabled: false,
			logger:  logger,
		}
	}

	// Create Application Insights client
	telemetryConfig := appinsights.NewTelemetryConfiguration(cfg.ConnectionString)

	// Configure telemetry settings
	telemetryConfig.MaxBatchSize = 8192
	telemetryConfig.MaxBatchInterval = 2 * time.Second

	client := appinsights.NewTelemetryClientFromConfig(telemetryConfig)

	logger.Info("Application Insights initialized successfully",
		zap.Bool("enabled", true))

	return &Client{
		client:  client,
		enabled: true,
		logger:  logger,
	}
}

// TrackRequest tracks an HTTP request
func (c *Client) TrackRequest(name string, url string, duration time.Duration, responseCode string, success bool) {
	if !c.enabled {
		return
	}

	request := appinsights.NewRequestTelemetry(name, url, duration, responseCode)
	request.Success = success
	c.client.Track(request)
}

// TrackException tracks an exception/error
func (c *Client) TrackException(err error) {
	if !c.enabled {
		return
	}

	exception := appinsights.NewExceptionTelemetry(err)
	c.client.Track(exception)
}

// TrackEvent tracks a custom event
func (c *Client) TrackEvent(name string, properties map[string]string, measurements map[string]float64) {
	if !c.enabled {
		return
	}

	event := appinsights.NewEventTelemetry(name)
	if properties != nil {
		event.Properties = properties
	}
	if measurements != nil {
		event.Measurements = measurements
	}
	c.client.Track(event)
}

// TrackMetric tracks a custom metric
func (c *Client) TrackMetric(name string, value float64) {
	if !c.enabled {
		return
	}

	metric := appinsights.NewMetricTelemetry(name, value)
	c.client.Track(metric)
}

// TrackTrace tracks a trace message
func (c *Client) TrackTrace(message string) {
	if !c.enabled {
		return
	}

	trace := appinsights.NewTraceTelemetry(message, appinsights.Information)
	c.client.Track(trace)
}

// Flush flushes all pending telemetry
func (c *Client) Flush() {
	if !c.enabled {
		return
	}

	select {
	case <-c.client.Channel().Close(10 * time.Second):
		c.logger.Info("Application Insights telemetry flushed successfully")
	case <-time.After(15 * time.Second):
		c.logger.Warn("Application Insights telemetry flush timed out")
	}
}

// IsEnabled returns whether Application Insights is enabled
func (c *Client) IsEnabled() bool {
	return c.enabled
}
