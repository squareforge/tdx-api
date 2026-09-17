# Hikyuu Data Service

独立的 hikyuu 数据下载服务，负责初始化 hikyuu 配置、执行全量/盘后增量下载，并提供任务状态 API。

默认端口：`8091`

Docker 数据目录：

- `/root/stocks`：hikyuu HDF5 数据和 `stock.db`
- `/root/.hikyuu`：hikyuu 配置文件
- `/app/logs`：下载任务日志

API：

- `GET /api/hikyuu/health`
- `GET /api/hikyuu/metadata`
- `GET /api/hikyuu/quality`
- `GET /api/hikyuu/kline?code=600519.SH&type=day&limit=120&recover=qfq`
- `POST /api/hikyuu/kline/batch`：批量读取最多 128 只证券，逐证券返回数据或错误
- `POST /api/hikyuu/indicators`
- `POST /api/hikyuu/backtest`
- `POST /api/hikyuu/tasks/full-sync`
- `POST /api/hikyuu/tasks/after-close-sync`
- `GET /api/hikyuu/tasks`
- `GET /api/hikyuu/tasks/{task_id}`

请求体可覆盖默认开关：

```json
{
  "day": true,
  "min": true,
  "min5": true,
  "trans": false,
  "time": false,
  "stock": true,
  "fund": true,
  "weight": true,
  "finance": true,
  "block": true,
  "use_tdx_number": 10
}
```

默认同步范围：

- 市场：沪深北
- 品种：股票、基金/ETF
- 周期：日线、1 分钟、5 分钟
- 扩展数据：权息、历史财务、板块、10 年期国债收益率

服务支持内置定时器；Compose 部署中由 selection-worker 的固定收盘任务在交易日
`16:30` 发起同步并等待完成，因此设置 `HIKYUU_SCHEDULER_ENABLED=false`，避免重复写入。

最多允许 `HIKYUU_MAX_CONCURRENT_TASKS`（默认 2）个并行下载任务：数据范围重叠的任务会被拒绝（409），
无重叠的任务可以同时运行（例如日线同步期间并行补充板块数据）。K 线 HDF5 写入通过文件锁串行化，
代码表/财务/板块等 SQLite 写入通过加长的 busy 超时等待重试；代码表导入由最早的任务负责，
并行任务自动跳过，避免重复写入。
查询、指标和参考回测使用短生命周期子进程读取 HDF5；普通读取会在服务内串行执行，
同步期间则返回 503，由上游 market-service 使用通达信兜底，避免读进程长期占用 HDF5 锁。

K 线查询支持 `day`、`minute1`、`minute5`、`week`、`month`，复权参数支持
`none`、`qfq`/`forward`、`hfq`/`backward` 以及等比复权。查询接口只读取本地
hikyuu 数据，不会触发下载；没有数据时返回失败，由上游 market-service 回退到
通达信。策略和回测始终通过 market-service 读取，不直接访问 Hikyuu 或旧 SQLite。

指标接口支持 `ma`、`ema`、`macd`、`boll`、`atr`，返回 `data_revision` 和
`calculation_engine` 便于复现研究结果。回测接口提供可校验的 Hikyuu MA 交叉
参考策略，现有 Go 策略回测仍保留作为主流程。
