# Selection Worker

指标选股服务，负责 Cron 调度、公式/策略选股运行、运行记录写入和通知触发。

服务源码和 Go module 位于当前目录，直接依赖 `packages/tdx-core`，不会再编译 `apps/web`。

默认端口：`8082`

Web 容器保留管理入口，Docker 部署下可以通过本服务独立执行选股任务。

宿主机默认映射端口为 `18082`。

历史回测每次运行都会写一份计时日志到 `data/logs/historical-backtests/<回测ID>.jsonl`。
日志按行记录加载阶段、每个交易日及每个策略的公式请求、股票评估、信号保存和组合模拟耗时；
`formula.worker_tick_ms` 是公式服务报告的计算耗时，`formula.request_ms` 包含请求传输和等待。
该目录位于已挂载的 `data/` 下，Docker 重启后仍可从宿主机读取。

源码运行：

```bash
cd services/selection-worker
go run .
```

可选环境变量：

- `PORT`：监听端口，默认 `8082`
- `FORMULA_WORKER_URL`：公式引擎地址，默认 `http://localhost:8712`
- `MARKET_SERVICE_URL`：行情服务地址。配置后，K 线、指数、代码、板块、财务、F10 和系统同步默认通过 `market-service` 获取
- `SELECTION_MARKET_FALLBACK_DIRECT`：行情服务失败时是否回退到 worker 本地 TDX 连接，默认 `false`；未配置 `MARKET_SERVICE_URL` 时会自动使用本地连接
- `AUTOMATION_SCHEDULER_ENABLED`：是否启用 Cron 调度，默认 `true`
- `SELECTION_QUOTE_MONITOR_ENABLED`：是否启用行情 SSE 监测和阈值告警，默认 `true`

## 行情职责

Docker 部署时建议保持 `MARKET_SERVICE_URL=http://market-service:8081`，并将
`SELECTION_MARKET_FALLBACK_DIRECT=false`。这样 `selection-worker` 只负责调度、公式选股、
策略执行和运行记录，通达信连接、行情读取以及后台 K 线/除权除息同步由 `market-service`
统一处理。

启用行情监测后，worker 会订阅关注股票池以及带有 `plan_buy` / `stop_loss` 的决策备注，
触发阈值时通过已启用的 Webhook 发送 `quote.alert` 事件；最近告警可通过
`GET /api/quote-alerts` 查看。

本地源码调试时可以不配置 `MARKET_SERVICE_URL`，worker 会保留直接连接 TDX 的能力。
