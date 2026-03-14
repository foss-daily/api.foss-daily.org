package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/time/rate"
)

var statsDir = func() string {
	if d := os.Getenv("FOSS_DAILY_STATS_DIR"); d != "" {
		return d
	}
	return "/tmp/stats"
}()

func main() {
	mux := http.NewServeMux()
	initGeo()
	heavyLimiter := newIPLimiter(rate.Every(100*time.Millisecond), 20, 10*time.Minute)
	lightLimiter := newIPLimiter(rate.Every(20*time.Millisecond), 100, 10*time.Minute)
	batchLimiter := newIPLimiter(rate.Every(500*time.Millisecond), 5, 10*time.Minute)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "https://gitea.foss-daily.org/foss-daily/api.foss-daily.org", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})
	mux.Handle("/v1/echo", lightLimiter.middleware(http.HandlerFunc(headerHandler)))
	mux.Handle("/v1/uuid", lightLimiter.middleware(http.HandlerFunc(uuidHandler)))
	mux.HandleFunc("/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		if !env("FOSS_DAILY_PROD") {
			http.Error(w, "Disabled", http.StatusNotFound)
			return
		}
		serve("overall.png")(w, r)
	})
	mux.Handle("/v1/me", lightLimiter.middleware(http.HandlerFunc(ipHandler)))
	mux.Handle("/v1/geo/", heavyLimiter.middleware(http.HandlerFunc(geoHandler)))
	mux.Handle("/v1/geo/batch", batchLimiter.middleware(http.HandlerFunc(geoBatchHandler)))
	mux.HandleFunc("/v1/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		if !env("FOSS_DAILY_PROD") {
			http.Error(w, "Disabled", http.StatusNotFound)
			return
		}
		bandwidthHandler(w, r)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := &http.Server{
		Addr:           ":6969",
		Handler:        secureHeaders(mux),
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   10 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 13,
	}
	log.Fatal(srv.ListenAndServe())
}
