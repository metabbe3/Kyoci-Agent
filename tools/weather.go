package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WeatherTool gets current weather information using Open-Meteo API
type WeatherTool struct {
	client *http.Client
}

func NewWeatherTool() *WeatherTool {
	return &WeatherTool{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *WeatherTool) Name() string { return "get_weather" }

func (t *WeatherTool) Description() string {
	return "Get current weather for a city. Returns temperature, humidity, wind speed, and weather description."
}

func (t *WeatherTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"city": map[string]interface{}{
				"type":        "string",
				"description": "Name of the city (e.g., 'Jakarta', 'New York', 'Tokyo')",
			},
		},
		"required": []string{"city"},
	}
}

// GeoCodeResult represents the geocoding API response
type GeoCodeResult struct {
	Results []struct {
		Name      string  `json:"name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Country   string  `json:"country"`
	} `json:"results"`
}

// WeatherResult represents the weather API response
type WeatherResult struct {
	Current struct {
		Temperature2M     float64 `json:"temperature_2m"`
		RelativeHumidity2M int    `json:"relative_humidity_2m"`
		WindSpeed10M      float64 `json:"wind_speed_10m"`
		WeatherCode       int     `json:"weather_code"`
	} `json:"current"`
}

func (t *WeatherTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var params struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", fmt.Errorf("invalid parameters: %w", err)
	}

	city := strings.TrimSpace(params.City)
	if city == "" {
		return "", fmt.Errorf("city name is required")
	}

	// Step 1: Geocode the city to get coordinates
	geoURL := fmt.Sprintf("https://geocoding-api.open-meteo.com/v1/search?name=%s&count=1&language=en&format=json",
		url.QueryEscape(city))
	
	req, err := http.NewRequestWithContext(ctx, "GET", geoURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create geocoding request: %w", err)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("geocoding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("geocoding API returned status %d", resp.StatusCode)
	}

	var geoData GeoCodeResult
	if err := json.NewDecoder(resp.Body).Decode(&geoData); err != nil {
		return "", fmt.Errorf("failed to parse geocoding response: %w", err)
	}

	if len(geoData.Results) == 0 {
		return "", fmt.Errorf("city not found: %s", city)
	}

	location := geoData.Results[0]
	lat := location.Latitude
	lon := location.Longitude

	// Step 2: Get weather data for the coordinates
	weatherURL := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.6f&longitude=%.6f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code&timezone=auto",
		lat, lon)

	req, err = http.NewRequestWithContext(ctx, "GET", weatherURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create weather request: %w", err)
	}

	resp, err = t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("weather request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	var weatherData WeatherResult
	if err := json.NewDecoder(resp.Body).Decode(&weatherData); err != nil {
		return "", fmt.Errorf("failed to parse weather response: %w", err)
	}

	// Format the weather description
	description := getWeatherDescription(weatherData.Current.WeatherCode)

	// Build formatted output
	output := fmt.Sprintf("Weather in %s, %s:\n", location.Name, location.Country)
	output += fmt.Sprintf("  Temperature: %.1f°C\n", weatherData.Current.Temperature2M)
	output += fmt.Sprintf("  Humidity: %d%%\n", weatherData.Current.RelativeHumidity2M)
	output += fmt.Sprintf("  Wind Speed: %.1f km/h\n", weatherData.Current.WindSpeed10M)
	output += fmt.Sprintf("  Condition: %s", description)

	return output, nil
}

// getWeatherDescription converts WMO weather codes to human-readable descriptions
func getWeatherDescription(code int) string {
	descriptions := map[int]string{
		0:  "Clear sky",
		1:  "Mainly clear",
		2:  "Partly cloudy",
		3:  "Overcast",
		45: "Foggy",
		48: "Depositing rime fog",
		51: "Light drizzle",
		53: "Moderate drizzle",
		55: "Dense drizzle",
		61: "Slight rain",
		63: "Moderate rain",
		65: "Heavy rain",
		66: "Light freezing rain",
		67: "Heavy freezing rain",
		71: "Slight snow fall",
		73: "Moderate snow fall",
		75: "Heavy snow fall",
		77: "Snow grains",
		80: "Slight rain showers",
		81: "Moderate rain showers",
		82: "Violent rain showers",
		85: "Slight snow showers",
		86: "Heavy snow showers",
		95: "Thunderstorm",
		96: "Thunderstorm with slight hail",
		99: "Thunderstorm with heavy hail",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Unknown condition"
}