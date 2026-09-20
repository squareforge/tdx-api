package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

var (
	client          *tdx.Client
	manager         *tdx.Manage
	marketClient    = NewMarketServiceClient()
	startupWarnings []string
	marketRuntimeMu sync.Mutex
	managerCronOn   bool
)

func useMarketService() bool {
	return marketClient != nil && marketClient.Enabled()
}

func allowDirectMarketFallback() bool {
	return !useMarketService() || envBool("SELECTION_MARKET_FALLBACK_DIRECT", false)
}

func ensureLocalMarketRuntime() error {
	if !allowDirectMarketFallback() {
		return errors.New("selection-worker 已配置 MARKET_SERVICE_URL，未启用本地行情 fallback")
	}
	return initMarketRuntime(false, false)
}

func initMarketRuntime(startCron bool, syncData bool) error {
	marketRuntimeMu.Lock()
	defer marketRuntimeMu.Unlock()
	if client != nil {
		if startCron && syncData && manager != nil && !managerCronOn {
			// 新版 tdx.Manage 定时器按需惰性启动,无需显式 Start
			managerCronOn = true
		}
		return nil
	}

	var err error
	client, err = tdx.DialDefault(tdx.WithDebug(false))
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	log.Println("成功连接到通达信服务器")

	if err = os.MkdirAll(tdx.DefaultDatabaseDir, 0755); err != nil {
		log.Printf("创建数据目录失败: %v", err)
		startupWarnings = append(startupWarnings, fmt.Sprintf("创建数据目录失败: %v", err))
	}
	codes, err := tdx.NewCodesSqlite(tdx.WithCodesClient(client))
	if codes != nil {
		tdx.DefaultCodes = codes
	}
	if err != nil {
		log.Printf("初始化代码库失败: %v", err)
		startupWarnings = append(startupWarnings, fmt.Sprintf("初始化代码库失败: %v", err))
	} else if syncData {
		if err := codes.Update(); err != nil {
			log.Printf("更新代码库失败: %v", err)
			startupWarnings = append(startupWarnings, fmt.Sprintf("更新代码库失败: %v", err))
		} else {
			log.Printf("已加载股票代码，共 %d 条", len(codes.GetStocks())+len(codes.GetETFs())+len(codes.GetIndexes()))
		}
	}

	manager, err = tdx.NewManage(tdx.WithClients(4))
	if err != nil {
		log.Printf("初始化数据管理器失败，部分任务和交易日接口将不可用: %v", err)
		startupWarnings = append(startupWarnings, fmt.Sprintf("初始化数据管理器失败: %v", err))
		return nil
	}
	if syncData {
		if err := manager.Workday.Update(); err != nil {
			log.Printf("更新交易日数据失败: %v", err)
			startupWarnings = append(startupWarnings, fmt.Sprintf("更新交易日数据失败: %v", err))
		}
	}
	if startCron && syncData {
		// 新版 tdx.Manage 定时器按需惰性启动,无需显式 Start
		managerCronOn = true
	}
	return nil
}

func envBool(name string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	if value == "" {
		return defaultValue
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func envInt(name string, defaultValue int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return n
}

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func successResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

func errorResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(Response{
		Code:    -1,
		Message: message,
		Data:    nil,
	})
}

func handleGetServerStatus(w http.ResponseWriter, r *http.Request) {
	type ServerStatus struct {
		Status    string   `json:"status"`
		Connected bool     `json:"connected"`
		Ready     bool     `json:"ready"`
		Version   string   `json:"version"`
		Uptime    string   `json:"uptime"`
		Warnings  []string `json:"warnings,omitempty"`
	}

	statusText := "running"
	ready := true
	if len(startupWarnings) > 0 {
		statusText = "degraded"
		ready = false
	}

	successResponse(w, &ServerStatus{
		Status:    statusText,
		Connected: client != nil,
		Ready:     ready,
		Version:   appVersion(),
		Uptime:    "unknown",
		Warnings:  startupWarnings,
	})
}

func handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	status := "healthy"
	if len(startupWarnings) > 0 {
		status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   status,
		"time":     time.Now().Format(time.RFC3339),
		"warnings": startupWarnings,
	})
}

func parseBool(value string) bool {
	if value == "" {
		return false
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parsePositiveInt(value string) int {
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func parseWorkdayDate(value string) (time.Time, error) {
	layouts := []string{"20060102", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %s", value)
}

func getAllCodeModels() ([]*tdx.CodeModel, error) {
	if useMarketService() {
		models, err := marketClient.Codes(context.Background())
		if err == nil && len(models) > 0 {
			return models, nil
		}
		if localModels, localErr := loadHikyuuStockCodeModels(context.Background()); localErr == nil {
			return localModels, nil
		} else if err == nil {
			err = localErr
		}
		if !allowDirectMarketFallback() {
			return nil, err
		}
		log.Printf("从 market-service 获取代码失败，尝试本地 fallback: %v", err)
	}
	if err := ensureLocalMarketRuntime(); err != nil {
		return nil, err
	}
	if tdx.DefaultCodes != nil {
		list := []*tdx.CodeModel(nil)
		for _, m := range tdx.DefaultCodes.Iter() {
			list = append(list, m)
		}
		if len(list) > 0 {
			return list, nil
		}
	}
	if client == nil {
		return nil, errors.New("行情客户端未初始化")
	}

	aggregate := []*tdx.CodeModel{}
	for _, ex := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ} {
		resp, err := client.GetCodeAll(ex)
		if err != nil || resp == nil {
			if err != nil {
				log.Printf("从服务器获取代码失败(%s): %v", ex.String(), err)
			}
			continue
		}
		for _, v := range resp.List {
			aggregate = append(aggregate, &tdx.CodeModel{
				Name:      v.Name,
				Code:      v.Code,
				Exchange:  ex.String(),
				Multiple:  v.Multiple,
				Decimal:   v.Decimal,
				LastPrice: v.LastPrice,
			})
		}
	}

	return aggregate, nil
}

func listenAndServe(defaultPort string) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	log.Printf("选股服务启动成功，访问 http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}
