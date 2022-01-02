package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"time"
)

const (
	FAKE_DATA_FILE = "testdata.json"
	CSS_URL        = "static/main.css"
	FAVICON_URL    = "https://openweathermap.org/img/wn/04d@2x.png"
	MAX_DATA_AGE   = "5m"
	HOURS_TO_SHOW  = 8
)

type WeatherLengthError string

func (w WeatherLengthError) Error() string {
	return fmt.Sprintf("weather length error: %s", string(w))
}

type Weather struct {
	Dt        int     `json:"dt"`
	Temp      float64 `json:"temp"`
	FeelsLike float64 `json:"feels_like"`
	Weather   []struct {
		Description string `json:"description"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Rain map[string]float64 `json:"rain"`
}

type Data struct {
	Timezone string    `json:"timezone"`
	Current  Weather   `json:"current"`
	Hourly   []Weather `json:"hourly"`
}

type HourlyState struct {
	Dt          string
	Temp        float64
	Icon        string
	Description string
	Rain        bool
}

type State struct {
	Temp        float64
	FeelsLike   float64
	Dt          string
	Description string
	Icon        string
	Hourly      []HourlyState
}

type Server struct {
	State      State
	LastLoaded time.Time
}

func (s *Server) UpdateState() {
	maxAge, err := time.ParseDuration(MAX_DATA_AGE)
	if err != nil {
		panic(err)
	}
	now := time.Now()
	if now.Sub(s.LastLoaded) > maxAge {
	}
}

func (s *Server) configHandler(c *gin.Context) {
	c.JSON(200, gin.H{
		"fake-data-file": FAKE_DATA_FILE,
	})
}

func (s *Server) fakeHandler(c *gin.Context) {
	reloaderState := struct {
		CssUrl  string
		Favicon string
	}{
		CSS_URL, FAVICON_URL,
	}
	c.HTML(http.StatusOK, "reloader.tmpl", reloaderState)
}

func (s *Server) fakeInner(c *gin.Context) {
	data := DataFromFile()
	state := &State{}
	state.Update(data)
	log.Printf("state = %#v", *state)
	c.HTML(http.StatusOK, "inner.tmpl", state)
}

func DataFromFile() *Data {
	data := Data{}
	fh, err := os.Open(FAKE_DATA_FILE)
	if err != nil {
		panic(err)
	}
	jsonBytes, err := ioutil.ReadAll(fh)
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		panic(err)
	}
	return &data
}

func (s *State) Update(data *Data) {
	if len(data.Current.Weather) < 1 {
		panic(WeatherLengthError("data.Current.Weather is empty"))
	}
	currentTime := time.Unix(int64(data.Current.Dt), 0)
	s.Temp = math.Round(data.Current.Temp)
	s.FeelsLike = math.Round(data.Current.FeelsLike)
	s.Description = data.Current.Weather[0].Description
	s.Icon = data.Current.Weather[0].Icon
	s.Dt = currentTime.Format("Mon 3:04 PM")

	hours := 0
	for i, h := range data.Hourly {
		if h.Dt <= data.Current.Dt {
			continue
		}
		hours++
		if hours > HOURS_TO_SHOW {
			break
		}

		hourlyTime := time.Unix(int64(h.Dt), 0)
		if len(data.Current.Weather) < 1 {
			panic(WeatherLengthError(fmt.Sprintf("data.hourly[%d].Current.Weather is empty", i)))
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
}

func main() {
	server := Server{}
	r := gin.Default()
	r.SetTrustedProxies([]string{})
	r.LoadHTMLGlob("templates/*.tmpl")
	r.GET("/config", server.configHandler)
	r.GET("/fake", server.fakeHandler)
	r.GET("/inner", server.fakeInner)
	r.Static("/static", "./static")
	r.Run()
}
