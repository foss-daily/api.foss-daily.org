package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "interest-cohort=()")
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func serve(file string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := filepath.Join(statsDir, filepath.Base(file))
		if _, err := os.Stat(p); os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.ServeFile(w, r, p)
	}
}

func firstEntry(traffic map[string]any, key string) map[string]any {
	arr, _ := traffic[key].([]any)
	if len(arr) == 0 {
		return nil
	}
	m, _ := arr[0].(map[string]any)
	return m
}

func rxTx(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	v, _ := m[key].(float64)
	return int64(v)
}

func humanize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatRxTx(rx, tx int64) string {
	return fmt.Sprintf("↓ %s / ↑ %s", humanize(rx), humanize(tx))
}

func env(key string) bool {
	return os.Getenv(key) == "1"
}

func realIP(r *http.Request) string {
	header := os.Getenv("FOSS_DAILY_IP_HEADER")
	if header == "" {
		header = "X-Forwarded-For"
	}
	ip := r.Header.Get(header)
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
		return ip
	}
	return strings.TrimSpace(strings.Split(ip, ",")[0])
}
