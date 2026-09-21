package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func main() {
	probeLogin := flag.Bool("probe-login", false, "run one RT_LOGIN probe and exit")
	flag.Parse()

	cfg := loadConfig()
	if *probeLogin {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(len(defaultLoginVariants())+1))
		defer cancel()
		result := ProbeLogin(ctx, ProbeRequest{
			Addr:     cfg.Addr,
			Username: cfg.Username,
			Password: cfg.Password,
			Timeout:  cfg.Timeout,
			Mode:     "auto",
		})
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			log.Fatal(err)
		}
		if !result.Success {
			os.Exit(1)
		}
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, response{Code: 0, Message: "success", Data: map[string]interface{}{
			"service": "quote-core-service",
			"addr":    cfg.Addr,
		}})
	})
	mux.HandleFunc("/api/probe/login", func(w http.ResponseWriter, r *http.Request) {
		req := ProbeRequest{
			Addr:     firstNonEmpty(r.URL.Query().Get("addr"), cfg.Addr),
			Username: firstNonEmpty(r.URL.Query().Get("user"), cfg.Username),
			Password: firstNonEmpty(r.URL.Query().Get("password"), cfg.Password),
			Timeout:  cfg.Timeout,
			Mode:     firstNonEmpty(r.URL.Query().Get("mode"), "auto"),
		}
		ctx, cancel := context.WithTimeout(r.Context(), req.Timeout*time.Duration(len(defaultLoginVariants())+1))
		defer cancel()
		result := ProbeLogin(ctx, req)
		code := 0
		msg := "success"
		if !result.Success {
			code = -1
			msg = "login probe failed"
		}
		writeJSON(w, response{Code: code, Message: msg, Data: result})
	})

	addr := ":" + cfg.Port
	log.Printf("quote-core-service listening on %s, tcp target=%s", addr, cfg.Addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

type config struct {
	Port     string
	Addr     string
	Username string
	Password string
	Timeout  time.Duration
}

func loadConfig() config {
	timeout, err := time.ParseDuration(firstNonEmpty(os.Getenv("QUOTE_CORE_TIMEOUT"), "5s"))
	if err != nil {
		timeout = 5 * time.Second
	}
	return config{
		Port:     firstNonEmpty(os.Getenv("PORT"), "8084"),
		Addr:     firstNonEmpty(os.Getenv("QUOTE_CORE_ADDR"), "39.98.157.150:8090"),
		Username: firstNonEmpty(os.Getenv("QUOTE_CORE_USER"), "anyone"),
		Password: firstNonEmpty(os.Getenv("QUOTE_CORE_PASSWORD"), "anyone"),
		Timeout:  timeout,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("write response failed: %v", err)
	}
}
