package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

const (
	CURRENT_URL_FMT  = "https://api.openweathermap.org/data/2.5/weather?lat=%f&lon=%f&appid=%s&units=%s"
	FORECAST_URL_FMT = "https://api.openweathermap.org/data/2.5/forecast?lat=%f&lon=%f&appid=%s&units=%s"
	CONFIG_FILE      = "config.json"
	FAKE_DATA_FILE   = "testdata.json"
	CSS_URL          = "static/main.css"
	FAVICON_URL      = "https://openweathermap.org/img/wn/04d@2x.png"
	INNER_URL        = "/inner"
	FAKE_INNER_URL   = "/fake-inner"

	MAX_DATA_AGE  = time.Duration(5) * time.Minute
	HOURS_TO_SHOW = 8
)

type WeatherLengthError string

func (w WeatherLengthError) Error() string {
	return fmt.Sprintf("weather length error: %s", string(w))
}

type Config struct {
	ApiKey      string  `json:"api_key"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Units       string  `json:"units"`
	TimezoneStr string  `json:"tz"`

	// Parsed TimezoneStr
	Timezone *time.Location
}

type WeatherCond struct {
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

type Weather struct {
	Dt        int                `json:"dt"`
	Temp      float64            `json:"temp"`
	FeelsLike float64            `json:"feels_like"`
	Weather   []WeatherCond      `json:"weather"`
	Rain      map[string]float64 `json:"rain"`
}

type owmMain struct {
	Temp      float64 `json:"temp"`
	FeelsLike float64 `json:"feels_like"`
}

type owmWeather struct {
	Dt      int                `json:"dt"`
	Main    owmMain            `json:"main"`
	Weather []WeatherCond      `json:"weather"`
	Rain    map[string]float64 `json:"rain"`
}

type owmForecast struct {
	List []owmWeather `json:"list"`
}

type Data struct {
	Current Weather   `json:"current"`
	Hourly  []Weather `json:"hourly"`

	// Cache-managemenent metadata
	CacheKey string
}

type HourlyState struct {
	Dt          string
	Temp        float64
	Icon        string
	Description string
	Rain        bool
}

type State struct {
	// Display state
	Temp        float64
	FeelsLike   float64
	Dt          string
	Description string
	Icon        string
	Hourly      []HourlyState

	// Cache-managemenent metadata
	Expiration time.Time
	CacheKey   string
}

type Server struct {
	state  State
	config *Config
}

func LoadConfig() *Config {
	config := Config{}
	fh, err := os.Open(CONFIG_FILE)
	if err != nil {
		panic(err)
	}
	jsonBytes, err := io.ReadAll(fh)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(jsonBytes, &config); err != nil {
		panic(err)
	}
	tz, err := time.LoadLocation(config.TimezoneStr)
	if err != nil {
		panic(err)
	}
	config.Timezone = tz
	log.Printf("Loaded config: %#v", config)
	return &config
}

func (s *Server) isStateExpired(cacheKey string) bool {
	now := time.Now()
	return now.After(s.state.Expiration) || cacheKey != s.state.CacheKey
}

func (s *Server) State(fakeData bool) *State {
	location := CurrentUrl(s.config, fakeData)
	if !s.isStateExpired(location) {
		log.Printf("Cached state from %s good till %s", location, s.state.Expiration)
		return &s.state
	}
	log.Println("state is expired")

	var data *Data
	start := time.Now()
	if fakeData {
		data = DataFromFile(location)
	} else {
		data = DataFromServer(s.config)
	}
	end := time.Now()
	log.Println("Loaded data server in", end.Sub(start))

	s.state.Update(data, s.config)
	log.Printf("New state from %s, expires %s = %#v", location, s.state.Expiration, s.state)
	return &s.state
}

func (s *Server) mainHandler(c *gin.Context) {
	reloaderState := struct {
		CssUrl   string
		Favicon  string
		InnerUrl string
	}{
		CSS_URL, FAVICON_URL, INNER_URL,
	}
	c.HTML(http.StatusOK, "reloader.tmpl", reloaderState)
}

func (s *Server) innerHandler(c *gin.Context) {
	state := s.State(false)
	c.HTML(http.StatusOK, "inner.tmpl", state)
}

func (s *Server) fakeHandler(c *gin.Context) {
	reloaderState := struct {
		CssUrl   string
		Favicon  string
		InnerUrl string
	}{
		CSS_URL, FAVICON_URL, FAKE_INNER_URL,
	}
	c.HTML(http.StatusOK, "reloader.tmpl", reloaderState)
}

func (s *Server) fakeInnerHandler(c *gin.Context) {
	state := s.State(true)
	c.HTML(http.StatusOK, "inner.tmpl", state)
}

func ForecastUrl(config *Config) string {
	return fmt.Sprintf(FORECAST_URL_FMT, config.Lat, config.Lon, config.ApiKey, config.Units)
}

func CurrentUrl(config *Config, fakeData bool) string {
	if fakeData {
		return FAKE_DATA_FILE
	}
	return fmt.Sprintf(CURRENT_URL_FMT, config.Lat, config.Lon, config.ApiKey, config.Units)
}

func DataFromFile(filename string) *Data {
	data := Data{CacheKey: filename}
	fh, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	jsonBytes, err := io.ReadAll(fh)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		panic(err)
	}
	return &data
}

func DataFromServer(config *Config) *Data {
	client := http.Client{Timeout: time.Duration(10) * time.Second}

	fetch := func(url string, out interface{}) {
		log.Println("fetch: url =", url)
		res, err := client.Get(url)
		if err != nil {
			panic(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			panic(err)
		}
		if res.StatusCode != http.StatusOK {
			panic(fmt.Sprintf("openweathermap returned %s: %s", res.Status, body))
		}
		if err := json.Unmarshal(body, out); err != nil {
			panic(err)
		}
	}

	current := &owmWeather{}
	forecast := &owmForecast{}
	currentUrl := CurrentUrl(config, false)
	fetch(currentUrl, current)
	fetch(ForecastUrl(config), forecast)

	data := &Data{CacheKey: currentUrl}
	data.Current = Weather{
		Dt:        current.Dt,
		Temp:      current.Main.Temp,
		FeelsLike: current.Main.FeelsLike,
		Weather:   current.Weather,
	}
	for _, h := range forecast.List {
		data.Hourly = append(data.Hourly, Weather{
			Dt:      h.Dt,
			Temp:    h.Main.Temp,
			Weather: h.Weather,
			Rain:    h.Rain,
		})
	}
	return data
}

func (s *State) Update(data *Data, config *Config) {
	if len(data.Current.Weather) < 1 {
		panic(WeatherLengthError("data.Current.Weather is empty"))
	}
	currentTime := time.Unix(int64(data.Current.Dt), 0).In(config.Timezone)
	s.Temp = math.Round(data.Current.Temp)
	s.FeelsLike = math.Round(data.Current.FeelsLike)
	s.Description = data.Current.Weather[0].Description
	s.Icon = data.Current.Weather[0].Icon
	s.Dt = currentTime.Format("Mon 3:04 PM")

	hours := 0
	s.Hourly = s.Hourly[:0]
	for i, h := range data.Hourly {
		if h.Dt <= data.Current.Dt {
			continue
		}
		hours++
		if hours > HOURS_TO_SHOW {
			break
		}

		hourlyTime := time.Unix(int64(h.Dt), 0).In(config.Timezone)
		if len(h.Weather) < 1 {
			panic(WeatherLengthError(fmt.Sprintf("data.hourly[%d].Weather is empty", i)))
		}
		hourly := HourlyState{
			Dt:          hourlyTime.Format("3:04 PM"),
			Temp:        math.Round(h.Temp),
			Icon:        h.Weather[0].Icon,
			Description: h.Weather[0].Description,
			Rain:        len(h.Rain) > 0,
		}
		s.Hourly = append(s.Hourly, hourly)
	}
	s.Expiration = time.Now().Add(MAX_DATA_AGE)
	s.CacheKey = data.CacheKey
}

func main() {
	server := Server{config: LoadConfig()}
	r := gin.Default()
	r.SetTrustedProxies([]string{})
	r.LoadHTMLGlob("templates/*.tmpl")
	r.GET("/", server.mainHandler)
	r.GET("/inner", server.innerHandler)
	r.GET("/fake", server.fakeHandler)
	r.GET("/fake-inner", server.fakeInnerHandler)
	r.Static("/static", "./static")
	r.Run()
}
