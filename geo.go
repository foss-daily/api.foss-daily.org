package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
)

var (
	cityDB     *geoip2.Reader
	asnDB      *geoip2.Reader
	geoEnabled bool
)

var resolveHostname = env("FOSS_DAILY_RESOLVE_HOSTNAME")

func initGeo() {
	cityPath := os.Getenv("FOSS_DAILY_CITY_DB")
	if cityPath == "" {
		cityPath = "/usr/local/share/GeoLite2/GeoLite2-City.mmdb"
	}
	asnPath := os.Getenv("FOSS_DAILY_ASN_DB")
	if asnPath == "" {
		asnPath = "/usr/local/share/GeoLite2/GeoLite2-ASN.mmdb"
	}

	if fi, err := os.Stat(cityPath); err != nil || fi.Size() == 0 {
		log.Printf("geo: city db unavailable (%s), geo endpoint disabled", cityPath)
		return
	}
	if fi, err := os.Stat(asnPath); err != nil || fi.Size() == 0 {
		log.Printf("geo: asn db unavailable (%s), geo endpoint disabled", asnPath)
		return
	}

	c, err := geoip2.Open(cityPath)
	if err != nil {
		log.Printf("geo: failed to open city db: %v, geo endpoint disabled", err)
		return
	}
	a, err := geoip2.Open(asnPath)
	if err != nil {
		c.Close()
		log.Printf("geo: failed to open asn db: %v, geo endpoint disabled", err)
		return
	}

	cityDB = c
	asnDB = a
	geoEnabled = true
	log.Printf("geo: both dbs loaded ok")
}

type GeoResponse struct {
	IP            string `json:"ip"`
	Hostname      string `json:"hostname,omitempty"`
	City          string `json:"city,omitempty"`
	Region        string `json:"region,omitempty"`
	Country       string `json:"country,omitempty"`
	CountryCode   string `json:"country_code,omitempty"`
	Continent     string `json:"continent,omitempty"`
	ContinentCode string `json:"continent_code,omitempty"`
	Loc           string `json:"loc,omitempty"`
	Postal        string `json:"postal,omitempty"`
	Timezone      string `json:"timezone,omitempty"`
	ASN           string `json:"asn,omitempty"`
	ASName        string `json:"as_name,omitempty"`
	ASDomain      string `json:"as_domain,omitempty"`
}

type cacheEntry struct {
	data    *GeoResponse
	expires time.Time
}

const (
	cacheTTL     = 24 * time.Hour
	cacheMaxSize = 100_000
	memHardLimit = 1536 * 1024 * 1024
	memSoftLimit = 1280 * 1024 * 1024
	evictBatch   = 5_000
)

var (
	geoCache   = make(map[string]*cacheEntry, cacheMaxSize)
	geoCacheMu sync.RWMutex
)

func heapBytes() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapInuse
}

func evictCache(n int) {
	evicted := 0
	for k := range geoCache {
		delete(geoCache, k)
		evicted++
		if evicted >= n {
			break
		}
	}
}

func init() {
	go func() {
		for range time.Tick(30 * time.Minute) {
			now := time.Now()
			geoCacheMu.Lock()
			for k, v := range geoCache {
				if now.After(v.expires) {
					delete(geoCache, k)
				}
			}
			if heapBytes() > memSoftLimit {
				log.Printf("cache: soft mem limit hit, evicting %d entries", evictBatch)
				evictCache(evictBatch)
				runtime.GC()
			}
			geoCacheMu.Unlock()
		}
	}()
}

func cacheGet(ip string) *GeoResponse {
	geoCacheMu.RLock()
	defer geoCacheMu.RUnlock()
	e, ok := geoCache[ip]
	if !ok || time.Now().After(e.expires) {
		return nil
	}
	return e.data
}

func cacheSet(ip string, data *GeoResponse) {
	if heapBytes() > memHardLimit {
		return
	}
	geoCacheMu.Lock()
	defer geoCacheMu.Unlock()
	if len(geoCache) >= cacheMaxSize {
		evictCache(evictBatch)
	}
	geoCache[ip] = &cacheEntry{data: data, expires: time.Now().Add(cacheTTL)}
}

func lookupGeo(ipStr string) (*GeoResponse, error) {
	if cached := cacheGet(ipStr); cached != nil {
		return cached, nil
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid ip")
	}
	resp := &GeoResponse{IP: ipStr}
	if cityDB != nil {
		r, err := cityDB.City(ip)
		if err == nil {
			resp.City = r.City.Names["en"]
			if len(r.Subdivisions) > 0 {
				resp.Region = r.Subdivisions[0].Names["en"]
			}
			resp.Country = r.Country.Names["en"]
			resp.CountryCode = r.Country.IsoCode
			resp.Continent = r.Continent.Names["en"]
			resp.ContinentCode = r.Continent.Code
			resp.Postal = r.Postal.Code
			resp.Timezone = r.Location.TimeZone
			if r.Location.Latitude != 0 || r.Location.Longitude != 0 {
				resp.Loc = fmt.Sprintf("%.4f,%.4f", r.Location.Latitude, r.Location.Longitude)
			}
		}
	}
	if asnDB != nil {
		r, err := asnDB.ASN(ip)
		if err == nil {
			resp.ASN = fmt.Sprintf("AS%d", r.AutonomousSystemNumber)
			resp.ASName = r.AutonomousSystemOrganization
		}
	}
	if resolveHostname {
		hosts, err := net.LookupAddr(ipStr)
		if err == nil && len(hosts) > 0 {
			resp.Hostname = strings.TrimSuffix(hosts[0], ".")
		}
	}
	cacheSet(ipStr, resp)
	return resp, nil
}

func geoHandler(w http.ResponseWriter, r *http.Request) {
	if !geoEnabled {
		http.Error(w, "geo endpoint unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/geo/")
	parts := strings.SplitN(path, "/", 2)
	ipStr := parts[0]
	field := ""
	if len(parts) == 2 {
		field = parts[1]
	}
	if ipStr == "me" || ipStr == "" {
		ipStr = realIP(r)
	}
	geo, err := lookupGeo(ipStr)
	if err != nil {
		http.Error(w, "invalid ip", http.StatusBadRequest)
		return
	}
	if field != "" {
		w.Header().Set("Content-Type", "text/plain")
		switch field {
		case "city":
			fmt.Fprint(w, geo.City)
		case "region":
			fmt.Fprint(w, geo.Region)
		case "country":
			fmt.Fprint(w, geo.Country)
		case "country_code":
			fmt.Fprint(w, geo.CountryCode)
		case "continent":
			fmt.Fprint(w, geo.Continent)
		case "continent_code":
			fmt.Fprint(w, geo.ContinentCode)
		case "loc":
			fmt.Fprint(w, geo.Loc)
		case "postal":
			fmt.Fprint(w, geo.Postal)
		case "timezone":
			fmt.Fprint(w, geo.Timezone)
		case "asn":
			fmt.Fprint(w, geo.ASN)
		case "as_name":
			fmt.Fprint(w, geo.ASName)
		default:
			http.Error(w, "unknown field", http.StatusNotFound)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(geo)
}

func geoBatchHandler(w http.ResponseWriter, r *http.Request) {
	if !geoEnabled {
		http.Error(w, "geo endpoint unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var ips []string
	if err := json.NewDecoder(r.Body).Decode(&ips); err != nil || len(ips) == 0 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(ips) > 100 {
		ips = ips[:100]
	}
	result := make(map[string]*GeoResponse, len(ips))
	for _, ip := range ips {
		geo, err := lookupGeo(ip)
		if err != nil {
			continue
		}
		result[ip] = geo
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
