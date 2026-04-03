package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
    _ "embed"
)

//go:embed docs/swagger.json
var swaggerJSON []byte

var (
	bwCache     []byte
	bwCacheTime time.Time
	bwMu        sync.Mutex
)

type iface struct {
	Name  string `json:"name"`
	Today string `json:"today"`
	Month string `json:"month"`
	Year  string `json:"year"`
}

// @Summary Get API version
// @Produce plain
// @Success 200 {string} string "version string"
// @Failure 405 {string} string "method not allowed"
// @Router /version [get]
func versionHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    w.Header().Set("Content-Type", "text/plain")
    fmt.Fprint(w, version)
}

// @Summary Generate UUID v4
// @Produce plain
// @Success 200 {string} string "uuid"
// @Failure 405 {string} string "method not allowed"
// @Router /uuid [get]
func uuidHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, uuid)
}

// @Summary Get your IP
// @Produce plain
// @Success 200 {string} string "ip address"
// @Failure 405 {string} string "method not allowed"
// @Router /me [get]
func ipHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, realIP(r))
}

// @Summary Echo your request headers
// @Produce plain
// @Success 200 {string} string "headers"
// @Failure 405 {string} string "method not allowed"
// @Router /echo [get]
func headerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	skip := map[string]bool{
		"Cookie":         true,
		"Cf-Ray":         true,
		"Cf-Ipcountry":   true,
		"Cf-Visitor":     true,
		"Cf-Warp-Tag-Id": true,
		"Cdn-Loop":       true,
	}
	w.Header().Set("Content-Type", "text/plain")
	keys := make([]string, 0, len(r.Header))
	for k := range r.Header {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s: %s\n", k, strings.Join(r.Header[k], ", "))
	}
}

// @Summary Get bandwidth stats
// @Produce json
// @Param plain query string false "set 1 for human readable"
// @Success 200 {object} iface
// @Failure 404 {string} string "Disabled"
// @Failure 405 {string} string "method not allowed"
// @Failure 500 {string} string "internal server error"
// @Router /bandwidth [get]
func bandwidthHandler(w http.ResponseWriter, r *http.Request) {
	if !env("FOSS_DAILY_PROD") {
        http.Error(w, "Disabled", http.StatusNotFound)
        return
    }
	ifaceName := os.Getenv("FOSS_DAILY_IFACE")
	if ifaceName == "" {
		ifaceName = "em0"
	}
	plain := r.URL.Query().Get("plain") == "1"
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bwMu.Lock()
	var err error
	if time.Since(bwCacheTime) > 1*time.Hour || bwCache == nil {
		out, err := exec.Command("vnstat", "-i", ifaceName, "--json").Output()
		if err != nil {
			bwMu.Unlock()
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		bwCache = out
		bwCacheTime = time.Now()
	}
	out := bwCache
	bwMu.Unlock()
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	interfaces, _ := raw["interfaces"].([]any)
	var result iface
	for _, i := range interfaces {
		m, _ := i.(map[string]any)
		name, ok := m["name"].(string)
		if !ok {
			continue
		}
		if name != ifaceName {
			continue
		}
		traffic, _ := m["traffic"].(map[string]any)
		day := firstEntry(traffic, "day")
		month := firstEntry(traffic, "month")
		year := firstEntry(traffic, "year")
		if plain {
			result = iface{
				Name:  "FOSS-Daily! Stats!",
				Today: fmt.Sprintf("down %s / up %s", humanize(rxTx(day, "rx")), humanize(rxTx(day, "tx"))),
				Month: fmt.Sprintf("down %s / up %s", humanize(rxTx(month, "rx")), humanize(rxTx(month, "tx"))),
				Year:  fmt.Sprintf("down %s / up %s", humanize(rxTx(year, "rx")), humanize(rxTx(year, "tx"))),
			}
		} else {
			result = iface{
				Name:  "FOSS-Daily! Stats!",
				Today: formatRxTx(rxTx(day, "rx"), rxTx(day, "tx")),
				Month: formatRxTx(rxTx(month, "rx"), rxTx(month, "tx")),
				Year:  formatRxTx(rxTx(year, "rx"), rxTx(year, "tx")),
			}
		}
		break
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache-Age", fmt.Sprintf("%ds", int(time.Since(bwCacheTime).Seconds())))
	json.NewEncoder(w).Encode(result)
}

// @Summary Get OpenAPI spec
// @Produce json
// @Success 200
// @Failure 405 {string} string "method not allowed"
// @Router /swagger [get]
func swaggerHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.Write(swaggerJSON)
}

// @Summary Bandwidth usage graph
// @Produce png
// @Success 200 {file} binary
// @Failure 404 {string} string "Disabled"
// @Failure 405 {string} string "method not allowed"
// @Router /usage [get]
func usageHandler(w http.ResponseWriter, r *http.Request) {
    if !env("FOSS_DAILY_PROD") {
        http.Error(w, "Disabled", http.StatusNotFound)
        return
    }
	if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    serve("overall.png")(w, r)
}