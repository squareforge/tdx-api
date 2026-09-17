package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend"
	"github.com/injoyai/tdx/protocol"
)

var (
	client          *tdx.Client
	manager         *tdx.Manage
	marketStore     *MarketStore
	hikyuuClient    *HikyuuDataServiceClient
	taskManager     = NewTaskManager()
	startupWarnings []string
	marketRuntimeMu sync.Mutex
	managerCronOn   bool
)

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
	// 连接通达信服务器
	client, err = tdx.DialDefault(tdx.WithDebug(false))
	if err != nil {
		return fmt.Errorf("连接服务器失败: %w", err)
	}
	log.Println("成功连接到通达信服务器")
	hikyuuClient = NewHikyuuDataServiceClient()
	if hikyuuClient.Enabled() {
		log.Printf("已配置 hikyuu 数据服务: %s", hikyuuClient.baseURL)
	}
	initMarketProviders()

	// 初始化代码缓存
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

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// 返回成功响应
func successResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// 返回错误响应
func errorResponse(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(Response{
		Code:    -1,
		Message: message,
		Data:    nil,
	})
}

// 获取五档行情
func handleGetQuote(w http.ResponseWriter, r *http.Request) {
	codeParam := r.URL.Query().Get("code")
	if codeParam == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}

	symbols, err := normalizeCodeParam(codeParam)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	if len(symbols) == 0 {
		errorResponse(w, "股票代码不能为空")
		return
	}

	codes := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		codes = append(codes, symbol.TDXCode)
	}
	quotes, err := client.GetQuote(codes...)
	if err != nil {
		errorResponse(w, fmt.Sprintf("获取行情失败: %v", err))
		return
	}

	successResponse(w, quotes)
}

// 获取K线数据（日K线默认使用前复权）
func handleGetKline(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	klineType := r.URL.Query().Get("type") // minute1/minute5/minute15/minute30/hour/day/week/month
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}

	var resp *protocol.KlineResp
	var err error

	if hikyuuResp, hikyuuErr := fetchHikyuuKline(r.Context(), code, klineType, 0); hikyuuErr == nil && hikyuuResp != nil && len(hikyuuResp.List) > 0 {
		successResponse(w, hikyuuResp)
		return
	} else if hikyuuErr != nil && hikyuuClient != nil && hikyuuClient.Enabled() {
		log.Printf("hikyuu K线查询失败(%s,%s)，回退原有数据源: %v", code, klineType, hikyuuErr)
	}

	switch klineType {
	case "minute1":
		// 分钟K线不需要复权
		resp, err = client.GetKlineMinuteAll(code)
	case "minute5":
		resp, err = client.GetKline5MinuteAll(code)
	case "minute15":
		resp, err = client.GetKline15MinuteAll(code)
	case "minute30":
		resp, err = client.GetKline30MinuteAll(code)
	case "hour":
		resp, err = client.GetKlineHourAll(code)
	case "week":
		// 周K线使用前复权（从日K线转换）
		resp, err = getQfqKlineDay(code)
		if err == nil && len(resp.List) > 0 {
			// 将日K线转换为周K线（简化版：每5个交易日合并）
			resp = convertToWeekKline(resp)
		}
	case "month":
		// 月K线使用前复权（从日K线转换）
		resp, err = getQfqKlineDay(code)
		if err == nil && len(resp.List) > 0 {
			// 将日K线转换为月K线
			resp = convertToMonthKline(resp)
		}
	case "day":
		fallthrough
	default:
		// 本地Hikyuu不可用时，回源通达信原始日线应急返回。
		resp, err = loadTDXKline(r.Context(), code, "day", 0)
	}

	if err != nil {
		errorResponse(w, fmt.Sprintf("获取K线失败: %v", err))
		return
	}

	successResponse(w, resp)
}

// getQfqKlineDay 获取前复权日K线数据
func getQfqKlineDay(code string) (*protocol.KlineResp, error) {
	// 使用同花顺API获取前复权数据
	klines, err := extend.GetTHSDayKline(code, extend.THS_QFQ)
	if err != nil {
		return nil, fmt.Errorf("获取前复权数据失败: %w", err)
	}

	if len(klines) == 0 {
		return nil, fmt.Errorf("同花顺前复权数据为空")
	}

	// 转换为 protocol.KlineResp 格式
	resp := &protocol.KlineResp{
		Count: uint16(len(klines)),
		List:  make([]*protocol.Kline, 0, len(klines)),
	}

	for i, k := range klines {
		pk := &protocol.Kline{
			Time:   k.Time,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
			Amount: k.Amount,
		}
		// 设置昨收价（使用上一条K线的收盘价）
		if i > 0 {
			pk.Last = klines[i-1].Close
		}
		resp.List = append(resp.List, pk)
	}

	return resp, nil
}

// convertToWeekKline 将日K线转换为周K线（简化版）
func convertToWeekKline(dayKline *protocol.KlineResp) *protocol.KlineResp {
	if len(dayKline.List) == 0 {
		return dayKline
	}

	weekResp := &protocol.KlineResp{
		List: make([]*protocol.Kline, 0),
	}

	var currentWeek *protocol.Kline
	var lastWeekDay time.Time

	for _, k := range dayKline.List {
		year, week := k.Time.ISOWeek()

		// 判断是否是新的一周
		if currentWeek == nil || lastWeekDay.Year() != year || getISOWeek(lastWeekDay) != week {
			// 保存上一周的数据
			if currentWeek != nil {
				weekResp.List = append(weekResp.List, currentWeek)
			}
			// 创建新周
			currentWeek = &protocol.Kline{
				Time:   k.Time,
				Last:   k.Last,
				Open:   k.Open,
				High:   k.High,
				Low:    k.Low,
				Close:  k.Close,
				Volume: k.Volume,
				Amount: k.Amount,
			}
		} else {
			// 累积当周数据
			if k.High > currentWeek.High {
				currentWeek.High = k.High
			}
			if k.Low < currentWeek.Low || currentWeek.Low == 0 {
				currentWeek.Low = k.Low
			}
			currentWeek.Close = k.Close
			currentWeek.Volume += k.Volume
			currentWeek.Amount += k.Amount
			currentWeek.Time = k.Time // 使用最后一天的时间
		}
		lastWeekDay = k.Time
	}

	// 添加最后一周
	if currentWeek != nil {
		weekResp.List = append(weekResp.List, currentWeek)
	}

	weekResp.Count = uint16(len(weekResp.List))
	return weekResp
}

// convertToMonthKline 将日K线转换为月K线
func convertToMonthKline(dayKline *protocol.KlineResp) *protocol.KlineResp {
	if len(dayKline.List) == 0 {
		return dayKline
	}

	monthResp := &protocol.KlineResp{
		List: make([]*protocol.Kline, 0),
	}

	var currentMonth *protocol.Kline
	var lastMonthKey string

	for _, k := range dayKline.List {
		monthKey := k.Time.Format("200601") // YYYYMM

		// 判断是否是新的一月
		if currentMonth == nil || lastMonthKey != monthKey {
			// 保存上一月的数据
			if currentMonth != nil {
				monthResp.List = append(monthResp.List, currentMonth)
			}
			// 创建新月
			currentMonth = &protocol.Kline{
				Time:   k.Time,
				Last:   k.Last,
				Open:   k.Open,
				High:   k.High,
				Low:    k.Low,
				Close:  k.Close,
				Volume: k.Volume,
				Amount: k.Amount,
			}
		} else {
			// 累积当月数据
			if k.High > currentMonth.High {
				currentMonth.High = k.High
			}
			if k.Low < currentMonth.Low || currentMonth.Low == 0 {
				currentMonth.Low = k.Low
			}
			currentMonth.Close = k.Close
			currentMonth.Volume += k.Volume
			currentMonth.Amount += k.Amount
			currentMonth.Time = k.Time // 使用最后一天的时间
		}
		lastMonthKey = monthKey
	}

	// 添加最后一月
	if currentMonth != nil {
		monthResp.List = append(monthResp.List, currentMonth)
	}

	monthResp.Count = uint16(len(monthResp.List))
	return monthResp
}

// getISOWeek 获取ISO周数
func getISOWeek(t time.Time) int {
	_, week := t.ISOWeek()
	return week
}

// 获取分时数据
func handleGetMinute(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	date := r.URL.Query().Get("date")
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}

	resp, usedDate, err := getMinuteWithFallback(code, date)
	if err != nil {
		errorResponse(w, fmt.Sprintf("获取分时数据失败: %v", err))
		return
	}

	if resp == nil {
		successResponse(w, map[string]interface{}{
			"date":  usedDate,
			"Count": 0,
			"List":  []interface{}{},
		})
		return
	}

	successResponse(w, map[string]interface{}{
		"date":  usedDate,
		"Count": resp.Count,
		"List":  resp.List,
	})
}

// 获取分时成交
func handleGetTrade(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	date := r.URL.Query().Get("date")
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}

	var resp *protocol.TradeResp
	var err error

	if date == "" {
		// 获取今日分时成交（最近1800条）
		resp, err = client.GetMinuteTrade(code, 0, 1800)
	} else {
		// 获取历史某天的分时成交
		resp, err = client.GetHistoryMinuteTradeDay(date, code)
	}

	if err != nil {
		errorResponse(w, fmt.Sprintf("获取分时成交失败: %v", err))
		return
	}

	successResponse(w, resp)
}

// 搜索股票代码
func handleSearchCode(w http.ResponseWriter, r *http.Request) {
	keyword := r.URL.Query().Get("keyword")
	if keyword == "" {
		errorResponse(w, "搜索关键词不能为空")
		return
	}

	keywordUpper := strings.ToUpper(keyword)
	results := []map[string]string{}
	seen := map[string]struct{}{}

	codeModels, err := getAllCodeModels()
	if err != nil {
		errorResponse(w, "搜索失败: "+err.Error())
		return
	}

	for _, model := range codeModels {
		fullCode := model.FullCode()
		if !protocol.IsStock(fullCode) && !protocol.IsETF(fullCode) {
			continue
		}
		if _, ok := seen[model.Code]; ok {
			continue
		}

		codeUpper := strings.ToUpper(model.Code)
		nameUpper := strings.ToUpper(model.Name)
		if strings.Contains(codeUpper, keywordUpper) || strings.Contains(nameUpper, keywordUpper) {
			securityType := "股票"
			if protocol.IsETF(fullCode) {
				securityType = "ETF"
			}
			results = append(results, map[string]string{
				"code":     model.Code,
				"name":     model.Name,
				"exchange": strings.ToLower(model.Exchange),
				"type":     securityType,
			})
			seen[model.Code] = struct{}{}
		}

		if len(results) >= 50 {
			break
		}
	}

	successResponse(w, results)
}

// 获取股票基本信息（整合多个接口）
func handleGetStockInfo(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		errorResponse(w, "股票代码不能为空")
		return
	}

	// 整合多个数据源
	result := make(map[string]interface{})

	// 1. 获取五档行情
	quotes, err := client.GetQuote(code)
	if err == nil && len(quotes) > 0 {
		result["quote"] = quotes[0]
	}

	// 2. 获取最近30天的日K线（使用前复权）
	kline, err := getQfqKlineDay(code)
	if err == nil && len(kline.List) > 30 {
		// 只返回最近30条
		kline.List = kline.List[len(kline.List)-30:]
		kline.Count = 30
	}
	if err == nil {
		result["kline_day"] = kline
	}

	// 3. 获取今日分时数据
	minute, minuteDate, err := getMinuteWithFallback(code, "")
	if err == nil && minute != nil {
		result["minute"] = map[string]interface{}{
			"date":  minuteDate,
			"Count": minute.Count,
			"List":  minute.List,
		}
	}

	successResponse(w, result)
}

func handleCreatePullKlineTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	if manager == nil {
		errorResponse(w, "数据管理器未初始化")
		return
	}

	var req struct {
		Codes     []string `json:"codes"`
		Tables    []string `json:"tables"`
		Dir       string   `json:"dir"`
		Limit     int      `json:"limit"`
		StartDate string   `json:"start_date"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}

	tables := req.Tables
	if len(tables) == 0 {
		tables = []string{extend.Day}
	} else {
		valid := make([]string, 0, len(tables))
		for _, v := range tables {
			if _, ok := extend.KlineTableMap[v]; ok {
				valid = append(valid, v)
			}
		}
		if len(valid) == 0 {
			errorResponse(w, "tables参数无效")
			return
		}
		tables = valid
	}

	dir := req.Dir
	if dir == "" {
		dir = filepath.Join(tdx.DefaultDatabaseDir, "kline")
	}

	startAt := time.Unix(0, 0)
	if req.StartDate != "" {
		var parsed bool
		for _, layout := range []string{"2006-01-02", "20060102"} {
			if t, err := time.ParseInLocation(layout, req.StartDate, time.Local); err == nil {
				startAt = t
				parsed = true
				break
			}
		}
		if !parsed {
			errorResponse(w, "start_date格式错误，应为YYYY-MM-DD或YYYYMMDD")
			return
		}
	}

	cfg := extend.PullKlineConfig{
		Codes:      req.Codes,
		Types:      tables,
		Dir:        dir,
		Goroutines: req.Limit,
		StartAt:    startAt,
	}

	puller, err := extend.NewPullKline(cfg)
	if err != nil {
		errorResponse(w, "初始化拉取器失败: "+err.Error())
		return
	}

	taskID := taskManager.Run("pull_kline", func(_ context.Context) error {
		// 新版 PullKline.Run 为定时任务入口,一次性拉取改用 Update
		return puller.Update(manager, true)
	})

	successResponse(w, map[string]string{
		"task_id": taskID,
	})
}

func handleCreateGbbqTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	if manager == nil {
		errorResponse(w, "数据管理器未初始化")
		return
	}

	taskID := taskManager.Run("sync_gbbq", func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		gbbq, err := tdx.NewGbbq(tdx.WithGbbqClient(client))
		if err != nil {
			return err
		}
		manager.Gbbq = gbbq
		return gbbq.Update()
	})

	successResponse(w, map[string]string{
		"task_id": taskID,
	})
}

func handleCreatePullTradeTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "只支持POST请求")
		return
	}
	if manager == nil {
		errorResponse(w, "数据管理器未初始化")
		return
	}

	var req struct {
		Code      string `json:"code"`
		Dir       string `json:"dir"`
		StartYear int    `json:"start_year"`
		EndYear   int    `json:"end_year"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "请求参数错误: "+err.Error())
		return
	}

	if req.Code == "" {
		errorResponse(w, "code不能为空")
		return
	}

	dir := req.Dir
	if dir == "" {
		dir = filepath.Join(tdx.DefaultDatabaseDir, "trade")
	}

	puller := extend.NewPullTrade(dir)

	taskID := taskManager.Run("pull_trade", func(ctx context.Context) error {
		if req.StartYear > 0 || req.EndYear > 0 {
			start, end := req.StartYear, req.EndYear
			if start <= 0 {
				start = 2000
			}
			if end <= 0 {
				end = time.Now().Year()
			}
			for year := start; year <= end; year++ {
				if err := puller.PullYear(ctx, manager, year, req.Code); err != nil {
					return err
				}
			}
			return nil
		}
		return puller.Pull(ctx, manager, req.Code)
	})

	successResponse(w, map[string]string{
		"task_id": taskID,
	})
}

func handleListTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}

	tasks := taskManager.List()
	successResponse(w, tasks)
}

func handleTaskOperations(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) == 2 && parts[1] == "cancel" {
		if r.Method != http.MethodPost {
			errorResponse(w, "取消任务仅支持POST")
			return
		}
		if ok := taskManager.Cancel(id); !ok {
			errorResponse(w, "任务不存在或已结束")
			return
		}
		successResponse(w, map[string]string{
			"task_id": id,
			"status":  string(TaskStatusCancelled),
		})
		return
	}

	if r.Method != http.MethodGet {
		errorResponse(w, "只支持GET请求")
		return
	}

	if task, ok := taskManager.Get(id); ok {
		successResponse(w, task)
		return
	}

	errorResponse(w, "任务不存在")
}

func splitCodes(param string) []string {
	parts := strings.Split(param, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		code := strings.TrimSpace(p)
		if code != "" {
			result = append(result, code)
		}
	}
	return result
}

func getMinuteWithFallback(code, date string) (*protocol.MinuteResp, string, error) {
	target := strings.TrimSpace(date)
	if target != "" {
		normalized, err := normalizeMinuteDate(target)
		if err != nil {
			return nil, target, err
		}
		resp, err := client.GetHistoryMinute(normalized, code)
		return resp, normalized, err
	}

	today := time.Now()
	todayStr := today.Format("20060102")
	resp, err := client.GetMinute(code)
	if err == nil && resp != nil && resp.Count > 0 && len(resp.List) > 0 {
		return resp, todayStr, nil
	}

	const maxLookback = 10

	lastResp := resp
	lastDate := todayStr
	lastErr := err

	for i := 0; i < maxLookback; i++ {
		currentDate := today.AddDate(0, 0, -i).Format("20060102")
		resp, err := client.GetHistoryMinute(currentDate, code)
		if err != nil {
			lastErr = err
			continue
		}
		if resp != nil {
			if len(resp.List) > 0 && resp.Count > 0 {
				return resp, currentDate, nil
			}
			if lastResp == nil {
				lastResp = resp
				lastDate = currentDate
			}
		}
	}

	if lastResp != nil {
		return lastResp, lastDate, nil
	}

	return nil, "", lastErr
}

func normalizeMinuteDate(date string) (string, error) {
	date = strings.TrimSpace(date)
	for _, layout := range []string{"20060102", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, date, time.Local); err == nil {
			return t.Format("20060102"), nil
		}
	}
	return "", fmt.Errorf("date 参数格式错误，应为 YYYYMMDD 或 YYYY-MM-DD")
}

func registerMarketRoutes() {
	http.HandleFunc("/api/quote", handleGetQuote)
	http.HandleFunc("/api/quote/standard", handleStandardQuote)
	http.HandleFunc("/api/stream/quotes", handleQuoteStream)
	http.HandleFunc("/api/kline", handleGetKline)
	http.HandleFunc("/api/minute", handleGetMinute)
	http.HandleFunc("/api/trade", handleGetTrade)
	http.HandleFunc("/api/search", handleSearchCode)
	http.HandleFunc("/api/stock-info", handleGetStockInfo)
	http.HandleFunc("/api/codes", handleGetCodes)
	http.HandleFunc("/api/batch-quote", handleBatchQuote)
	http.HandleFunc("/api/kline-history", handleGetKlineHistory)
	http.HandleFunc("/api/index", handleGetIndex)
	http.HandleFunc("/api/index/all", handleGetIndexAll)
	http.HandleFunc("/api/market-stats", handleGetMarketStats)
	http.HandleFunc("/api/market-count", handleGetMarketCount)
	http.HandleFunc("/api/stock-codes", handleGetStockCodes)
	http.HandleFunc("/api/etf-codes", handleGetETFCodes)
	http.HandleFunc("/api/server-status", handleGetServerStatus)
	http.HandleFunc("/api/health", handleHealthCheck)
	http.HandleFunc("/api/etf", handleGetETFList)
	http.HandleFunc("/api/trade-history", handleGetTradeHistory)
	http.HandleFunc("/api/trade-history/full", handleGetTradeHistoryFull)
	http.HandleFunc("/api/minute-trade-all", handleGetMinuteTradeAll)
	http.HandleFunc("/api/kline-all", handleGetKlineAllTDX)
	http.HandleFunc("/api/kline-all/tdx", handleGetKlineAllTDX)
	http.HandleFunc("/api/kline-all/ths", handleGetKlineAllTHS)
	http.HandleFunc("/api/workday", handleGetWorkday)
	http.HandleFunc("/api/workday/range", handleGetWorkdayRange)
	http.HandleFunc("/api/income", handleGetIncome)
	http.HandleFunc("/api/call-auction", handleGetCallAuction)
	http.HandleFunc("/api/gbbq", handleGetGbbq)
	http.HandleFunc("/api/finance", handleGetFinance)
	http.HandleFunc("/api/finance/standard", handleStandardFinance)
	http.HandleFunc("/api/news", handleNews)
	http.HandleFunc("/api/news/sync", handleNewsSync)
	http.HandleFunc("/api/long-tiger", handleLongTiger)
	http.HandleFunc("/api/market/research", handleMarketResearch)
	http.HandleFunc("/api/market/notice", handleMarketNotice)
	http.HandleFunc("/api/market/hot-money", handleMarketHotMoney)
	http.HandleFunc("/api/market/sync", handleMarketInfoSync)
	http.HandleFunc("/api/market/industry/research", handleIndustryResearch)
	http.HandleFunc("/api/market/industry/options", handleIndustryDict)
	http.HandleFunc("/api/market/industry/rank", handleIndustryRank)
	http.HandleFunc("/api/market/industry/money", handleIndustryMoneyRank)
	http.HandleFunc("/api/market/stock-money", handleStockMoneyRank)
	http.HandleFunc("/api/market/rzrq/rank", handleRzrqRank)
	http.HandleFunc("/api/market/rzrq/trend", handleRzrqTrend)
	http.HandleFunc("/api/market/trading-status", handleMarketTradingStatus)
	http.HandleFunc("/api/analysis/context", handleAnalysisContext)
	http.HandleFunc("/api/company/categories", handleGetCompanyCategories)
	http.HandleFunc("/api/company/content", handleGetCompanyContent)
	http.HandleFunc("/api/block", handleGetBlockData)
	http.HandleFunc("/api/tdx-hy", handleGetTdxHy)
	http.HandleFunc("/api/tdx-stat", handleGetTdxStat)
	http.HandleFunc("/api/tdx-stat2", handleGetTdxStat2)
	http.HandleFunc("/api/xgsg", handleGetXgsg)
	http.HandleFunc("/api/exhq/markets", handleExHqMarkets)
	http.HandleFunc("/api/exhq/count", handleExHqCount)
	http.HandleFunc("/api/exhq/instruments", handleExHqInstruments)
	http.HandleFunc("/api/exhq/quote", handleExHqQuote)
	http.HandleFunc("/api/exhq/bars", handleExHqBars)
	http.HandleFunc("/api/exhq/trade", handleExHqTrade)
	http.HandleFunc("/api/tasks/pull-kline", handleCreatePullKlineTask)
	http.HandleFunc("/api/tasks/sync-gbbq", handleCreateGbbqTask)
	http.HandleFunc("/api/tasks/pull-trade", handleCreatePullTradeTask)
	http.HandleFunc("/api/tasks", handleListTasks)
	http.HandleFunc("/api/tasks/", handleTaskOperations)
	http.HandleFunc("/api/hikyuu/kline/batch", handleHikyuuKlineBatch)
	http.HandleFunc("/api/hikyuu/freshness", handleHikyuuFreshness)
	http.HandleFunc("/api/hikyuu/tasks/after-close-sync", handleHikyuuAfterCloseSync)
	http.HandleFunc("/api/hikyuu/tasks/", handleHikyuuTask)
}

func listenAndServe(defaultPort string) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	log.Printf("服务启动成功，访问 http://localhost%s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func main() {
	var err error
	marketStore, err = openMarketStore()
	if err != nil {
		log.Fatal(err)
	}
	defer closeMarketStore()
	if err := initMarketRuntime(envBool("MARKET_SYNC_ON_START", true), envBool("MARKET_SYNC_ON_START", true)); err != nil {
		log.Fatal(err)
	}
	registerMarketRoutes()
	listenAndServe("8081")
}
