package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	quoteprotocol "quote-protocol"
)

type response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func main() {
	probeLogin := flag.Bool("probe-login", false, "run one RT_LOGIN probe and exit")
	listFunctions := flag.Bool("list-functions", false, "list RT_* function ids and exit")
	requestFunction := flag.String("request", "", "send one logged-in AskData request by function name or hex id, such as RT_SERVERTIME or 0x0902")
	expectFunction := flag.String("expect", "", "expected response function name or hex id; defaults to -request")
	payloadHex := flag.String("payload-hex", "", "hex payload appended after AskData header")
	requestSize := flag.Int("size", 0, "AskData m_nSize")
	requestOption := flag.Int("option", 0, "AskData m_nOption")
	quoteSymbols := flag.String("quote", "", "comma-separated symbols for RT_REALTIME_EXT_INT64, such as sh600570,sz000001")
	stockInfo := flag.Bool("stock-info", false, "query RT_STOCKINFOLIST and print parsed counts plus sample rows")
	stockInfoLimit := flag.Int("stock-info-limit", 20, "number of parsed RT_STOCKINFOLIST rows to print; use -1 for all rows")
	heartbeatTest := flag.Duration("heartbeat-test", 0, "start SDK long connection for the duration with 2s heartbeat interval, then print stats")
	flag.Parse()

	cfg := loadConfig()
	switch {
	case *listFunctions:
		writeStdoutJSON(quoteprotocol.FunctionCapabilities())
		return
	case *probeLogin:
		runProbeLogin(cfg)
		return
	case strings.TrimSpace(*requestFunction) != "":
		runGenericRequest(cfg, *requestFunction, *expectFunction, *payloadHex, *requestSize, *requestOption)
		return
	case strings.TrimSpace(*quoteSymbols) != "":
		runRealtime(cfg, *quoteSymbols)
		return
	case *stockInfo:
		runStockInfo(cfg, *stockInfoLimit)
		return
	case *heartbeatTest > 0:
		runHeartbeatTest(cfg, *heartbeatTest)
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

func runProbeLogin(cfg config) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*time.Duration(len(defaultLoginVariants())+1))
	defer cancel()
	result := ProbeLogin(ctx, ProbeRequest{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		Timeout:  cfg.Timeout,
		Mode:     "auto",
	})
	writeStdoutJSON(result)
	if !result.Success {
		os.Exit(1)
	}
}

func runGenericRequest(cfg config, requestFunction string, expectFunction string, payloadHex string, size int, option int) {
	requestType, err := quoteprotocol.ParseFunction(requestFunction)
	if err != nil {
		log.Fatalf("parse request function failed: %v", err)
	}
	expectType := requestType
	if strings.TrimSpace(expectFunction) != "" {
		expectType, err = quoteprotocol.ParseFunction(expectFunction)
		if err != nil {
			log.Fatalf("parse expect function failed: %v", err)
		}
	}
	var payload []byte
	if strings.TrimSpace(payloadHex) != "" {
		payload, err = hex.DecodeString(strings.TrimSpace(payloadHex))
		if err != nil {
			log.Fatalf("parse payload hex failed: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout+2*time.Second)
	defer cancel()
	client := quoteprotocol.New(quoteprotocol.Config{
		Addr:           cfg.Addr,
		Username:       cfg.Username,
		Password:       cfg.Password,
		DialTimeout:    cfg.Timeout,
		RequestTimeout: cfg.Timeout,
	})
	if err := client.Start(ctx); err != nil {
		writeStdoutJSON(map[string]interface{}{
			"addr":  cfg.Addr,
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer client.Close()
	resp, err := client.Request(ctx, quoteprotocol.Request{
		Type:       requestType,
		ExpectType: expectType,
		Size:       int16(size),
		Option:     int16(option),
		Payload:    payload,
	})
	result := map[string]interface{}{
		"addr":     cfg.Addr,
		"request":  formatFunctionLabel(requestType),
		"expected": formatFunctionLabel(expectType),
		"response": resp,
	}
	if err != nil {
		result["error"] = err.Error()
	}
	writeStdoutJSON(result)
	if err != nil {
		os.Exit(1)
	}
}

func runRealtime(cfg config, symbolsCSV string) {
	var symbols []string
	for _, symbol := range strings.Split(symbolsCSV, ",") {
		if symbol = strings.TrimSpace(symbol); symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout+2*time.Second)
	defer cancel()
	client := quoteprotocol.New(quoteprotocol.Config{
		Addr:           cfg.Addr,
		Username:       cfg.Username,
		Password:       cfg.Password,
		DialTimeout:    cfg.Timeout,
		RequestTimeout: cfg.Timeout,
	})
	if err := client.Start(ctx); err != nil {
		writeStdoutJSON(map[string]interface{}{
			"addr":  cfg.Addr,
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer client.Close()
	quotes, err := client.GetRealtime(ctx, symbols)
	result := map[string]interface{}{
		"addr":   cfg.Addr,
		"quotes": quotes,
	}
	if err != nil {
		result["error"] = err.Error()
	}
	writeStdoutJSON(result)
	if err != nil {
		os.Exit(1)
	}
}

func runStockInfo(cfg config, limit int) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := quoteprotocol.New(quoteprotocol.Config{
		Addr:           cfg.Addr,
		Username:       cfg.Username,
		Password:       cfg.Password,
		DialTimeout:    cfg.Timeout,
		RequestTimeout: cfg.Timeout,
	})
	if err := client.Start(ctx); err != nil {
		writeStdoutJSON(map[string]interface{}{
			"addr":  cfg.Addr,
			"error": err.Error(),
		})
		os.Exit(1)
	}
	defer client.Close()

	list, err := client.GetStockInfoList(ctx, nil)
	result := map[string]interface{}{
		"addr":  cfg.Addr,
		"total": len(list),
	}
	if err != nil {
		result["error"] = err.Error()
		writeStdoutJSON(result)
		os.Exit(1)
	}
	byType := map[string]int{}
	for _, item := range list {
		byType[item.CodeTypeHex]++
	}
	if limit < 0 || limit > len(list) {
		limit = len(list)
	}
	result["by_code_type"] = byType
	result["sample"] = list[:limit]
	writeStdoutJSON(result)
}

func runHeartbeatTest(cfg config, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), duration+cfg.Timeout+2*time.Second)
	defer cancel()
	client := quoteprotocol.New(quoteprotocol.Config{
		Addr:              cfg.Addr,
		Username:          cfg.Username,
		Password:          cfg.Password,
		DialTimeout:       cfg.Timeout,
		RequestTimeout:    cfg.Timeout,
		HeartbeatInterval: 2 * time.Second,
	})
	if err := client.Start(ctx); err != nil {
		writeStdoutJSON(map[string]interface{}{
			"addr":  cfg.Addr,
			"error": err.Error(),
		})
		os.Exit(1)
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
	stats := client.Stats()
	err := client.Close()
	result := map[string]interface{}{
		"addr":  cfg.Addr,
		"stats": stats,
	}
	if err != nil {
		result["close_error"] = err.Error()
	}
	writeStdoutJSON(result)
}

func formatFunctionLabel(value uint16) string {
	name := quoteprotocol.FunctionName(value)
	if name == "" {
		return quoteprotocol.HexType(value)
	}
	return fmt.Sprintf("%s(%s)", name, quoteprotocol.HexType(value))
}

func writeStdoutJSON(data interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "encode json failed: %v\n", err)
		os.Exit(1)
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
		Addr:     firstNonEmpty(os.Getenv("QUOTE_CORE_ADDR"), "222.66.65.68:8888"),
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
