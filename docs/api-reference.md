# API 参考

## 🌐 基础信息

**Base URL**: `http://localhost:8080`  
**Content-Type**: `application/json; charset=utf-8`  
**编码**: UTF-8

---

## 📋 响应格式

所有接口统一返回格式：

```json
{
  "code": 0,           // 0=成功, -1=失败
  "message": "success", // 提示信息
  "data": {}           // 数据内容
}
```

---

## 📊 API接口列表

### 1. 获取五档行情

**接口**: `GET /api/quote`

**描述**: 获取股票实时五档买卖盘口数据

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码（如：000001）支持多个，逗号分隔 |

**请求示例**:
```
GET /api/quote?code=000001
GET /api/quote?code=000001,600519
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "Exchange": 0,
      "Code": "000001",
      "Active1": 2843,
      "K": {
        "Last": 12250,    // 昨收价（厘）
        "Open": 12300,    // 开盘价（厘）
        "High": 12600,    // 最高价（厘）
        "Low": 12280,     // 最低价（厘）
        "Close": 12500    // 收盘价/最新价（厘）
      },
      "ServerTime": "1730617200",
      "TotalHand": 1235000,    // 总手数
      "Intuition": 100,        // 现量
      "Amount": 156000000,     // 成交额
      "InsideDish": 520000,    // 内盘
      "OuterDisc": 715000,     // 外盘
      "BuyLevel": [            // 买五档
        {
          "Buy": true,
          "Price": 12500,      // 买一价（厘）
          "Number": 35000      // 挂单量（股）
        },
        // ... 买二到买五
      ],
      "SellLevel": [           // 卖五档
        {
          "Buy": false,
          "Price": 12510,      // 卖一价（厘）
          "Number": 30000      // 挂单量（股）
        },
        // ... 卖二到卖五
      ],
      "Rate": 0.0,
      "Active2": 2843
    }
  ]
}
```

**数据说明**:
- 价格单位：厘（1元 = 1000厘）
- 成交量单位：手（1手 = 100股）
- 挂单量单位：股

---

### 2. 获取K线数据

**接口**: `GET /api/kline`

**描述**: 获取股票K线数据（OHLC + 成交量成交额）。日/周/月K线默认返回同花顺前复权数据；分钟级及小时级为通达信原始数据。需要原始数据可调用 `/api/kline-all/tdx`。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码（如：000001） |
| type | string | 否 | K线类型，默认day |

**K线类型(type)**:
- `minute1` - 1分钟K线（最多24000条）
- `minute5` - 5分钟K线
- `minute15` - 15分钟K线
- `minute30` - 30分钟K线
- `hour` - 60分钟/小时K线
- `day` - 日K线（默认）
- `week` - 周K线
- `month` - 月K线

**请求示例**:
```
GET /api/kline?code=000001&type=day
GET /api/kline?code=600519&type=minute30
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "Count": 100,
    "List": [
      {
        "Last": 12250,      // 昨收价（厘）
        "Open": 12300,      // 开盘价（厘）
        "High": 12600,      // 最高价（厘）
        "Low": 12280,       // 最低价（厘）
        "Close": 12500,     // 收盘价（厘）
        "Volume": 1235000,  // 成交量（手）
        "Amount": 156000000,// 成交额（厘）
        "Time": "2024-11-03T00:00:00Z",
        "UpCount": 0,       // 上涨数（指数有效）
        "DownCount": 0      // 下跌数（指数有效）
      }
      // ... 更多K线数据
    ]
  }
}
```

**数据说明**:
- 数据按时间倒序排列（最新的在前）
- 价格单位：厘
- 成交量单位：手
- 成交额单位：厘

---

### 2. 标准行情快照

**接口**: `GET /api/quote/standard`

**描述**: 获取带有标准股票代码、市场、数据源和采集时间的实时行情快照。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| codes | string | 是 | 股票代码，逗号分隔，最多50只 |

代码支持 `600519`、`SH600519`、`600519.SH` 等形式，标准返回格式为 `600519.SH`。

**请求示例**:
```text
GET /api/quote/standard?codes=600519.SH,000001.SZ
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "symbol": "600519.SH",
      "code": "600519",
      "exchange": "SH",
      "market": "CN_A",
      "source": "tdx",
      "fetched_at": "2026-08-29T15:00:00+08:00",
      "quote": {}
    }
  ]
}
```

---

### 3. SSE 行情订阅

**接口**: `GET /api/stream/quotes`

**描述**: 建立 SSE 长连接，后台按订阅股票批量轮询行情，只推送发生变化的报价。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| codes | string | 是 | 股票代码，逗号分隔，最多200只 |

**请求示例**:
```text
GET /api/stream/quotes?codes=600519.SH,000001.SZ
```

事件类型：

- `ready`：连接建立
- `quote`：行情变化，数据位于 `snapshot`
- `heartbeat`：连接保活

后台轮询间隔由 `MARKET_QUOTE_POLL_INTERVAL_SECONDS` 控制，默认3秒。

---

### 4. 获取分时数据

**接口**: `GET /api/minute`

**描述**: 获取股票分时走势数据。接口严格按照请求日期返回结果；若指定日期无数据，将返回空列表并保留请求日期。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码（如：000001） |
| date | string | 否 | 日期（YYYYMMDD格式），默认当天 |

**请求示例**:
```
GET /api/minute?code=000001
GET /api/minute?code=000001&date=20241103
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "date": "20241108",   // 实际数据日期
    "Count": 240,
    "List": [
      {
        "Time": "09:31",
        "Price": 12300,    // 价格（厘）
        "Number": 1500     // 成交量（手）
      },
      {
        "Time": "09:32",
        "Price": 12310,
        "Number": 1200
      }
      // ... 240个数据点（9:30-11:30, 13:00-15:00）
    ]
  }
}
```

**数据说明**:
- 交易时段：9:30-11:30（120分钟）, 13:00-15:00（120分钟）
- 共240个数据点
- 价格单位：厘
- 若 `List` 为空，表示该日期无分时数据，请由调用方自行选择备用日期或数据源

---

### 4. 获取分时成交

**接口**: `GET /api/trade`

**描述**: 获取股票逐笔成交明细

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码（如：000001） |
| date | string | 否 | 日期（YYYYMMDD格式），默认当天 |

**请求示例**:
```
GET /api/trade?code=000001
GET /api/trade?code=000001&date=20241103
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "Count": 1800,
    "List": [
      {
        "Time": "2024-11-03T14:59:58Z",
        "Price": 12500,    // 成交价（厘）
        "Volume": 100,     // 成交量（手）
        "Status": 0,       // 0=买入, 1=卖出, 2=中性
        "Number": 5        // 成交单数
      },
      {
        "Time": "2024-11-03T14:59:55Z",
        "Price": 12490,
        "Volume": 50,
        "Status": 1,
        "Number": 3
      }
      // ... 更多成交记录
    ]
  }
}
```

**数据说明**:
- Status: 0=主动买入(红色), 1=主动卖出(绿色), 2=中性
- 当日最多返回1800条
- 历史日期最多返回2000条
- 价格单位：厘
- 成交量单位：手

---

### 5. 搜索股票代码

**接口**: `GET /api/search`

**描述**: 根据关键词搜索股票代码和名称

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| keyword | string | 是 | 搜索关键词（代码或名称） |

**请求示例**:
```
GET /api/search?keyword=平安
GET /api/search?keyword=000001
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    {
      "code": "000001",
      "name": "平安银行"
    },
    {
      "code": "601318",
      "name": "中国平安"
    }
    // ... 最多50条结果
  ]
}
```

**数据说明**:
- 支持代码和名称模糊搜索
- 最多返回50条结果
- 仅返回A股（过滤指数等）

---

### 6. 获取股票综合信息

**接口**: `GET /api/stock-info`

**描述**: 一次性获取股票的多种数据（五档行情+日K线+分时）

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码（如：000001） |

**请求示例**:
```
GET /api/stock-info?code=000001
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "quote": {
      // 五档行情数据（同/api/quote）
    },
    "kline_day": {
      // 最近30天日K线（同/api/kline?type=day）
    },
    "minute": {
      // 今日分时数据（同/api/minute）
    }
  }
}
```

**数据说明**:
- 整合了五档行情、最近30条日K线、最新分时数据
- 分时数据自带 `date`、`Count`、`List` 字段；若 `List` 为空表示该日期无分时数据
- 分时数据自带 `date`、`Count`、`List` 字段，便于识别回退日期
- 适合快速获取股票概览，减少API调用次数

---

## 🔧 扩展接口（高级功能）

### 7. 获取股票列表

**接口**: `GET /api/codes`

**描述**: 获取指定交易所的所有股票代码列表

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| exchange | string | 否 | 交易所代码，默认all |

**交易所代码**:
- `sh` - 上海证券交易所
- `sz` - 深圳证券交易所
- `bj` - 北京证券交易所
- `all` - 全部（默认）

**请求示例**:
```
GET /api/codes
GET /api/codes?exchange=sh
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 5234,
    "exchanges": {
      "sh": 2156,
      "sz": 2845,
      "bj": 233
    },
    "codes": [
      {
        "code": "000001",
        "name": "平安银行",
        "exchange": "sz"
      }
      // ... 更多股票
    ]
  }
}
```

---

### 8. 批量获取行情

**接口**: `POST /api/batch-quote`

**描述**: 批量获取多只股票的实时行情

**请求参数** (JSON Body):
```json
{
  "codes": ["000001", "600519", "601318"]
}
```

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/batch-quote \
  -H "Content-Type: application/json" \
  -d '{"codes":["000001","600519","601318"]}'
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": [
    // 数组，每个元素同/api/quote的单个股票数据
  ]
}
```

---

### 9. 获取历史K线

**接口**: `GET /api/kline-history`

**描述**: 获取指定时间范围的K线数据

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 股票代码 |
| type | string | 是 | K线类型 |
| start_date | string | 否 | 开始日期（YYYYMMDD） |
| end_date | string | 否 | 结束日期（YYYYMMDD） |
| limit | int | 否 | 返回条数，默认100，最大800 |

**请求示例**:
```
GET /api/kline-history?code=000001&type=day&limit=30
GET /api/kline-history?code=000001&type=day&start_date=20241001&end_date=20241101
```

---

### 10. 获取指数数据

**接口**: `GET /api/index`

**描述**: 获取指数K线数据（如上证指数、深证成指）

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | string | 是 | 指数代码（如：sh000001） |
| type | string | 否 | K线类型，默认day |

**常用指数代码**:
- `sh000001` - 上证指数
- `sz399001` - 深证成指
- `sz399006` - 创业板指
- `sh000300` - 沪深300

**请求示例**:
```
GET /api/index?code=sh000001&type=day
```

---

### 11. 获取服务状态

**接口**: `GET /api/server-status`

**描述**: 返回API服务运行状态。

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "status": "running",
    "connected": true,
    "version": "0.1.2",
    "uptime": "unknown"
  }
}
```

---

### 12. 创建批量K线入库任务

**接口**: `POST /api/tasks/pull-kline`

**描述**: 启动后台任务，批量拉取指定股票、指定周期的K线数据并存入本地数据库（默认目录：`data/database/kline`）。任务在后台异步执行，可通过任务管理接口查询状态。

**请求参数**（JSON Body）:
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| codes | array | 否 | 股票代码数组，默认遍历全部A股 |
| tables | array | 否 | K线类型列表，取值见下表，默认 `["day"]` |
| dir | string | 否 | 数据库存储目录，默认 `data/database/kline` |
| limit | int | 否 | 并发协程数量，默认1 |
| start_date | string | 否 | 起始日期阈值（`YYYY-MM-DD` 或 `YYYYMMDD`），早于此日期的数据不会重新拉取 |

**K线类型列表**:
`minute`, `5minute`, `15minute`, `30minute`, `hour`, `day`, `week`, `month`, `quarter`, `year`

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/tasks/pull-kline \
  -H "Content-Type: application/json" \
  -d '{
    "codes": ["000001","600519"],
    "tables": ["day","week","month"],
    "limit": 4,
    "start_date": "2020-01-01"
  }'
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "9b0d1b1b-7c3d-4ce6-9a0e-bd9f5e0dcf3b"
  }
}
```

---

### 13. 创建股本变迁同步任务

**接口**: `POST /api/tasks/sync-gbbq`

**描述**: 在 `market-service` 中后台同步股本变迁、除权除息数据。返回的任务 ID
可以通过 `/api/tasks/{task_id}` 查询状态。

**请求示例**:
```bash
curl -X POST http://localhost:18081/api/tasks/sync-gbbq
```

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "task_id": "9b0d1b1b-7c3d-4ce6-9a0e-bd9f5e0dcf3b"
  }
}
```

---

### 14. 创建分时成交入库任务

**接口**: `POST /api/tasks/pull-trade`

**描述**: 拉取指定股票从 `start_year` 到 `end_year` 的历史分时成交数据，并自动导出CSV（默认目录：`data/database/trade`）。

**请求参数**（JSON Body）:
| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码（如：000001） |
| dir | string | 否 | 输出目录，默认 `data/database/trade` |
| start_year | int | 否 | 起始年份，默认2000 |
| end_year | int | 否 | 结束年份，默认当年 |

**请求示例**:
```bash
curl -X POST http://localhost:8080/api/tasks/pull-trade \
  -H "Content-Type: application/json" \
  -d '{
    "code": "000001",
    "start_year": 2015,
    "end_year": 2023
  }'
```

**响应示例**同上，返回 `task_id`。

---

### 15. 查询与控制任务

| 接口 | 方法 | 描述 |
|------|------|------|
| `/api/tasks` | GET | 列出所有已创建任务及状态 |
| `/api/tasks/{task_id}` | GET | 查询指定任务详情 |
| `/api/tasks/{task_id}/cancel` | POST | 取消正在执行的任务 |

**任务状态枚举**:
- `running`：执行中
- `success`：已完成
- `failed`：执行失败，`error` 字段包含原因
- `cancelled`：已取消

**响应示例** (`GET /api/tasks/{task_id}`):
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": "9b0d1b1b-7c3d-4ce6-9a0e-bd9f5e0dcf3b",
    "type": "pull_kline",
    "status": "running",
    "started_at": "2025-11-10T13:05:26.123456+08:00"
  }
}
```

---

### 16. 获取ETF列表

**接口**: `GET /api/etf`

**描述**: 返回当前可用的 ETF 基金列表，可按交易所过滤并限制返回数量。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| exchange | string | 否 | 交易所，`sh` / `sz` / `all`（默认） |
| limit | int | 否 | 返回条数限制 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 2,
    "list": [
      {
        "code": "510300",
        "name": "沪深300ETF",
        "exchange": "sh",
        "last_price": 4.123
      },
      {
        "code": "159915",
        "name": "创业板ETF",
        "exchange": "sz",
        "last_price": 1.876
      }
    ]
  }
}
```

---

### 17. 获取历史分时成交（分页）

**接口**: `GET /api/trade-history`

**描述**: 分页获取历史交易日的分时成交明细，单次最多返回 2000 条。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码 |
| date | string | 是 | 交易日期（YYYYMMDD） |
| start | int | 否 | 起始游标，默认0 |
| count | int | 否 | 返回条数，默认2000，最大2000 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "Count": 2000,
    "List": [
      {
        "Price": 12345,
        "Time": "2024-11-08T14:58:00+08:00",
        "Status": 0,
        "Volume": 50
      }
    ]
  }
}
```

---

### 18. 获取全天分时成交

**接口**: `GET /api/minute-trade-all`

**描述**: 一次性获取某交易日的全部分时成交明细；未指定日期时返回当日实时成交。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码 |
| date | string | 否 | 交易日期（YYYYMMDD），默认当天 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "Count": 3150,
    "List": [
      {
        "Price": 12500,
        "Time": "2024-11-08T09:30:01+08:00",
        "Volume": 10,
        "Status": 0
      }
    ]
  }
}
```

---

### 19. 查询交易日信息

**接口**: `GET /api/workday`

**描述**: 查询指定日期是否为交易日，并返回前后若干个最近的交易日。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| date | string | 否 | 查询日期（YYYYMMDD 或 YYYY-MM-DD），默认当天 |
| count | int | 否 | 返回的前后交易日数量，范围 1-30，默认1 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "date": {
      "iso": "2024-11-08",
      "numeric": "20241108"
    },
    "is_workday": true,
    "next": [
      {
        "iso": "2024-11-11",
        "numeric": "20241111"
      }
    ],
    "previous": [
      {
        "iso": "2024-11-07",
        "numeric": "20241107"
      }
    ]
  }
}
```

---

### 20. 获取市场证券数量

**接口**: `GET /api/market-count`

**描述**: 获取上交所、深交所、北交所当前可用证券数量统计。

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "total": 7654,
    "exchanges": [
      { "exchange": "sh", "count": 2163 },
      { "exchange": "sz", "count": 5337 },
      { "exchange": "bj", "count": 154 }
    ]
  }
}
```

---

### 21. 获取股票代码列表

**接口**: `GET /api/stock-codes`

**描述**: 返回全市场股票代码列表，可控制是否携带交易所前缀。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| limit | int | 否 | 返回条数限制 |
| prefix | bool | 否 | 是否包含交易所前缀（默认 true，即 `sh600000`） |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "count": 5600,
    "list": [
      "sh600000",
      "sz000001"
      // ...
    ]
  }
}
```

---

### 22. 获取ETF代码列表

**接口**: `GET /api/etf-codes`

**描述**: 返回所有 ETF 基金代码，参数与 `/api/stock-codes` 相同。

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "count": 200,
    "list": [
      "sh510050",
      "sz159915"
    ]
  }
}
```

---

### 23. 获取股票全部历史K线

**接口**: `GET /api/kline-all`

**描述**: 返回指定股票在某个周期的全部历史 K 线数据（天、周、月自动使用前复权）。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码 |
| type | string | 否 | K 线类型，默认 day，可选 minute1/5/15/30/hour/day/week/month/quarter/year |
| limit | int | 否 | 返回条数限制（从最近开始截取） |

**注意**: 全量数据较大，建议配合 `limit` 控制响应大小。

---

### 24. 获取指数全部历史K线

**接口**: `GET /api/index/all`

**描述**: 返回指数在各周期的全部历史 K 线数据。

**请求参数**与 `/api/kline-all` 相同。

---

### 25. 获取上市以来分时成交

**接口**: `GET /api/trade-history/full`

**描述**: 返回指定股票上市以来的全部历史分时成交明细，可选截断截止日期与限制数量。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码 |
| before | string | 否 | 截止日期（YYYYMMDD 或 YYYY-MM-DD），默认今日 |
| limit | int | 否 | 返回条数限制（从最近开始截取） |

---

### 26. 获取交易日范围

**接口**: `GET /api/workday/range`

**描述**: 返回指定起止日期之间的所有交易日。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| start | string | 是 | 起始日期（YYYYMMDD 或 YYYY-MM-DD） |
| end | string | 是 | 结束日期（YYYYMMDD 或 YYYY-MM-DD） |

---

### 27. 计算收益区间指标

**接口**: `GET /api/income`

**描述**: 以某日收盘价格为基准，计算若干交易日后的收益情况。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码 |
| start_date | string | 是 | 基准日期（YYYYMMDD 或 YYYY-MM-DD） |
| days | string | 否 | 多个天数偏移（逗号分隔），默认 5,10,20,60,120 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "count": 3,
    "list": [
      {
        "offset": 5,
        "time": "2024-11-15T15:00:00+08:00",
        "rise": 350.0,
        "rise_rate": 0.0285,
        "source": { "close": 12250.0, "open": 12300.0, "...": 0 },
        "current": { "close": 12580.0, "open": 12600.0, "...": 0 }
      }
    ]
  }
}
```

---

## 💡 使用示例

### Python示例

```python
import requests

BASE_URL = "http://localhost:8080"

# 1. 获取五档行情
def get_quote(code):
    url = f"{BASE_URL}/api/quote?code={code}"
    response = requests.get(url)
    data = response.json()
    if data['code'] == 0:
        return data['data']
    return None

# 2. 获取日K线
def get_kline(code, type='day'):
    url = f"{BASE_URL}/api/kline?code={code}&type={type}"
    response = requests.get(url)
    data = response.json()
    if data['code'] == 0:
        return data['data']['List']
    return None

# 3. 搜索股票
def search_stock(keyword):
    url = f"{BASE_URL}/api/search?keyword={keyword}"
    response = requests.get(url)
    data = response.json()
    if data['code'] == 0:
        return data['data']
    return None

# 使用示例
if __name__ == "__main__":
    # 搜索股票
    stocks = search_stock("平安")
    print(f"搜索结果: {stocks}")
    
    # 获取行情
    quote = get_quote("000001")
    print(f"最新价: {quote[0]['K']['Close'] / 1000}元")
    
    # 获取K线
    klines = get_kline("000001", "day")
    print(f"获取到{len(klines)}条K线数据")
```

### JavaScript示例

```javascript
const BASE_URL = 'http://localhost:8080';

// 1. 获取五档行情
async function getQuote(code) {
    const response = await fetch(`${BASE_URL}/api/quote?code=${code}`);
    const data = await response.json();
    if (data.code === 0) {
        return data.data;
    }
    return null;
}

// 2. 获取K线
async function getKline(code, type = 'day') {
    const response = await fetch(`${BASE_URL}/api/kline?code=${code}&type=${type}`);
    const data = await response.json();
    if (data.code === 0) {
        return data.data.List;
    }
    return null;
}

// 3. 批量获取行情
async function batchGetQuote(codes) {
    const response = await fetch(`${BASE_URL}/api/batch-quote`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json'
        },
        body: JSON.stringify({ codes })
    });
    const data = await response.json();
    return data.data;
}

// 使用示例
(async () => {
    // 获取行情
    const quote = await getQuote('000001');
    console.log('最新价:', quote[0].K.Close / 1000);
    
    // 获取K线
    const klines = await getKline('000001', 'day');
    console.log('K线数据量:', klines.length);
    
    // 批量获取
    const quotes = await batchGetQuote(['000001', '600519', '601318']);
    console.log('批量行情:', quotes.length);
})();
```

### cURL示例

```bash
# 1. 获取五档行情
curl "http://localhost:8080/api/quote?code=000001"

# 2. 获取日K线
curl "http://localhost:8080/api/kline?code=000001&type=day"

# 3. 获取分时数据
curl "http://localhost:8080/api/minute?code=000001"

# 4. 搜索股票
curl "http://localhost:8080/api/search?keyword=平安"

# 5. 批量获取行情
curl -X POST http://localhost:8080/api/batch-quote \
  -H "Content-Type: application/json" \
  -d '{"codes":["000001","600519"]}'
```

---

## 📚 全量历史K线接口

为了区分不同数据源，并方便调用方自行决定兜底策略，历史K线提供以下两个独立接口，返回格式完全一致：

### 1. 通达信原始历史K线

**接口**: `GET /api/kline-all/tdx`

**说明**: 返回通达信原始（不复权）K线，内部按800条一批拼接完成。支持所有 `type` 取值（分钟、小时、日、周、月、季、年）。

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| code | string | 是 | 股票代码（6位数字） |
| type | string | 否 | 默认 `day`，取值同 `/api/kline` |
| limit | int | 否 | 结果截断条数（从末尾取最近N条），默认返回全量 |

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "count": 4100,
    "list": [
      {
        "Time": "1991-04-03T00:00:00Z",
        "Open": 1260,
        "High": 1320,
        "Low": 1240,
        "Close": 1280,
        "Volume": 3500,
        "Amount": 4280000,
        "Last": 0
      }
      // ... 时间正序排列的全部K线
    ],
    "meta": {
      "source": "tdx",
      "type": "day",
      "batch_limit": 800,
      "notes": [
        "通达信单次底层请求最多返回 800 条数据，服务端已顺序拼接全量结果",
        "对于上市时间较长的标的，请预估调用耗时（通常 1-5 秒），客户端需自行设置超时与兜底策略",
        "若实测请求在超时阈值内成功返回数据，即视为成功调用，无需按预设超时上限计入统计"
      ]
    }
  }
}
```

### 2. 同花顺前复权历史K线

**接口**: `GET /api/kline-all/ths`

**说明**: 返回同花顺前复权日K线，并提供基于日K转换的周、月K线。仅支持 `type=day/week/month`。

**请求参数**: 同上，`type` 限于 `day`、`week`、`month`。

**响应示例**:
```json
{
  "code": 0,
  "message": "success",
  "data": {
    "count": 4100,
    "list": [
      {
        "Time": "1991-04-03T00:00:00Z",
        "Open": 1260,
        "High": 1320,
        "Low": 1240,
        "Close": 1280,
        "Volume": 3500,
        "Amount": 4280000,
        "Last": 0
      }
      // ... 全量前复权数据
    ],
    "meta": {
      "source": "ths",
      "type": "day",
      "batch_limit": 4100,
      "notes": [
        "同花顺接口一次性返回前复权数据，响应时长依赖网络与标的数据量（通常 2-8 秒）",
        "建议调用方在 Python 等客户端中设置 ≥10 秒超时时间，并按需准备自定义兜底逻辑",
        "若实测请求在超时阈值内成功返回数据，即视为成功调用，无需按预设超时上限计入统计"
      ]
    }
  }
}
```

> ⚠️ **提示**：上述接口不会对接第三方兜底逻辑；若返回空或失败，请由调用方自行决定重试或切换数据源。

---

## 🔒 错误码说明

| code | message | 说明 |
|------|---------|------|
| 0 | success | 请求成功 |
| -1 | 股票代码不能为空 | 缺少必填参数code |
| -1 | 获取行情失败: xxx | 数据获取失败，xxx为具体错误 |
| -1 | 获取K线失败: xxx | K线数据获取失败 |
| -1 | 未找到相关股票 | 搜索无结果 |
| -1 | 搜索关键词不能为空 | 缺少keyword参数 |

---

## 📊 数据单位换算

### 价格单位
- **返回值**：厘（1元 = 1000厘）
- **换算公式**：元 = 厘 / 1000
- **示例**：12500厘 = 12.50元

### 成交量单位
- **返回值**：手（1手 = 100股）
- **换算公式**：股 = 手 × 100
- **示例**：1235手 = 123500股

### 成交额单位
- **返回值**：厘
- **换算公式**：元 = 厘 / 1000
- **示例**：156000000厘 = 156000元 = 15.6万元

---

## 🚀 性能建议

1. **批量请求**：使用批量接口代替多次单个请求
2. **缓存**：对不常变化的数据（如股票列表）做本地缓存
3. **限流**：避免频繁请求，建议间隔>=3秒
4. **压缩**：使用gzip压缩减少传输量

---

## 上游能力补充接口

### AI 研究

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `POST /api/ai/research/stock` | 生成单股结构化研究报告；受控读取行情与 Hikyuu 证据 | `{"provider":"deepseek","symbol":"603171","input":{"name":"税友股份"}}` |
| `POST /api/ai/select/rank` | 对候选股票执行 Hikyuu MA 参考验证并由 AI 排序 | `{"provider":"deepseek","symbols":["000001","603171"],"input":{"fast_period":5,"slow_period":20}}` |

请求中的 `credential_id`、`provider`、`model`、`options` 与其他 AI 接口一致。响应 `data` 包含 `result`（结论、技术信号、证据链、宏观风险、策略适配、数据质量、下一步检查）以及 `prompt_version`、`data_revision`、`tools_used`。报告严格区分输入事实、指标计算结果和 AI 判断；数据缺失时返回 `unknown`，不补造行情或事件。

选股排序响应中的 `historical_validation` 来自 Hikyuu 参考回测，`score` 仅表示候选集内相对排序，不是未来收益概率。

### 标准行情扩展

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `GET /api/call-auction` | 集合竞价 | `/api/call-auction?code=000001` |
| `GET /api/gbbq` | 股本变迁/除权除息事件 | `/api/gbbq?code=600519` |
| `GET /api/finance` | 财务/基本面信息 | `/api/finance?code=600519` |
| `GET /api/company/categories` | F10 公司资料目录 | `/api/company/categories?code=600519` |
| `GET /api/company/content` | F10 公司资料正文 | `/api/company/content?code=600519&filename=xxx&start=0&length=1024` |

### 板块与配置

| 接口 | 说明 | 参数 |
| --- | --- | --- |
| `GET /api/block` | 板块成分，可选补板块指数代码 | `file=gn/fg/zs/hy/block`，`with_index=true/false` |
| `GET /api/tdx-hy` | 通达信/申万行业归属 | 无 |
| `GET /api/tdx-stat` | 个股综合统计 | 无 |
| `GET /api/tdx-stat2` | 资金流向与板块归属统计 | 无 |
| `GET /api/xgsg` | 新股申购列表 | 无 |

### 扩展行情 TdxExHq

扩展行情使用通达信 7727 端口，适合期货、港股、外盘等市场。

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `GET /api/exhq/markets` | 扩展市场代码表 | `/api/exhq/markets` |
| `GET /api/exhq/count` | 扩展品种数量 | `/api/exhq/count` |
| `GET /api/exhq/instruments` | 扩展品种列表分页 | `/api/exhq/instruments?start=0&count=100` |
| `GET /api/exhq/quote` | 单品种五档行情 | `/api/exhq/quote?market=31&code=HK00700` |
| `GET /api/exhq/bars` | 扩展 K 线 | `/api/exhq/bars?market=31&code=HK00700&category=9&start=0&count=100` |
| `GET /api/exhq/trade` | 扩展分笔成交 | `/api/exhq/trade?market=31&code=HK00700&start=0&count=200` |

`category` 使用通达信 K 线类型值，例如 `9` 为日线，`5` 为周线，`6` 为月线，`7` 为 1 分钟。

### 公式、选股与 HQChart

### 宏观事件与预警

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `GET /api/macro-events` | 事件列表，可按 `from`、`to`、`category`、`impact` 筛选 | `/api/macro-events?category=inflation&impact=high` |
| `POST /api/macro-events` | 新增事件，`starts_at` 使用 RFC3339 | `{"code":"PMI","name":"中国制造业 PMI","country":"CN","category":"china_macro","starts_at":"2026-09-30T09:30:00+08:00"}` |
| `PUT /api/macro-events/{id}` | 更新事件 | `/api/macro-events/macro-custom-1` |
| `POST /api/macro-events/{id}/acknowledge` | 标记或恢复已处理状态 | `{"acknowledged":true}` |
| `GET /api/macro-events/sync` | 查看 BLS、Federal Reserve、BEA 同步状态 | `/api/macro-events/sync` |
| `POST /api/macro-events/sync` | 手动同步官方日程 | `/api/macro-events/sync` |
| `GET /api/macro-events/settings` | 查看提醒窗口与 Webhook 设置 | `/api/macro-events/settings` |
| `PUT /api/macro-events/settings` | 保存提醒规则；`webhook_ids` 是 JSON 字符串 | `/api/macro-events/settings` |
| `GET /api/macro-events/overview` | 当前风险窗口及持仓/观察池摘要 | `/api/macro-events/overview` |

`macro_event.alert_due` 表示提前提醒，`macro_event.window_started` 表示风险窗口开始。通知默认关闭；开启后每个事件、渠道和提醒类型只发送一次。事件提醒只提示复核，不自动阻止交易。

### Hikyuu 研究数据

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `GET /api/hikyuu/health` | 服务、版本、数据修订和质量状态 | `/api/hikyuu/health` |
| `GET /api/hikyuu/metadata` | HDF5/SQLite 文件、体积和证券数量 | `/api/hikyuu/metadata` |
| `GET /api/hikyuu/quality` | 数据目录、OHLC、重复时间和序列检查；可带 `code`、`period` | `/api/hikyuu/quality?code=000001&period=day` |
| `POST /api/hikyuu/indicators` | Hikyuu 指标值：`ma`、`ema`、`macd`、`boll`、`atr` | `{"code":"000001","indicator":"macd","limit":800}` |
| `POST /api/hikyuu/backtest` | MA 交叉参考回测，用于和 Go 回测校验 | `{"symbols":["000001"],"fast":5,"slow":20}` |

研究响应中的 `meta.data_revision`、`meta.calculation_engine` 用于复现和解释计算来源。

| 接口 | 说明 | 示例 |
| --- | --- | --- |
| `GET /api/formula/health` | 查看公式 worker 和当前公式引擎状态 | `/api/formula/health` |
| `POST /api/formula/run` | 直接执行公式脚本 | `{"symbol":"000001","script":"T:MA(C,5);","out_count":3}` |
| `GET /api/formulas` | 公式列表 | `/api/formulas` |
| `POST /api/formulas/{id}/test` | 使用已保存公式测试单只股票 | `{"symbol":"000001","calc_count":80}` |
| `GET /api/stock-pools` | 股票池列表 | `/api/stock-pools` |
| `GET /api/automations` | 自动化任务列表 | `/api/automations` |
| `POST /api/automations/{id}/run` | 手动运行自动化任务 | `{}` |
| `GET /api/selection-results` | 查看选股命中结果 | `/api/selection-results?limit=100` |
| `GET /api/selection-results/tracking` | 计算并查看信号后 D1/D5/D10 的真实前向表现与聚合命中率；加 `cached=1` 只读取已缓存快照 | `/api/selection-results/tracking?latest=1&cached=1` |
| `POST /api/selection-results/tracking` | 手动刷新选股结果跟踪快照 | `/api/selection-results/tracking?limit=500` |
| `GET /api/webhooks` | Webhook 通知配置列表 | `/api/webhooks` |
| `GET /api/hqchart/kline` | Web 页面使用的专业行情 K 线对象 | `/api/hqchart/kline?symbol=000001&period=day&limit=500` |
| `GET /api/hqchart/history` | HQChart 原生历史 K 线数组适配 | `/api/hqchart/history?symbol=000001&period=day&limit=800` |

选股结果跟踪使用信号后的真实交易日 K 线计算开盘收益、收盘收益、区间最高涨幅和最大回撤。达标率是历史样本的确定性统计，不是未来成功概率；未满观察窗口的结果会标记为“待观察”。

`/api/formula/health` 在 Docker 模式下正常会返回 `engine=hqchartpy2` 和 `hqchartpy2_available=true`。如果 HQChartPy2 不可用，worker 会继续使用内置 fallback 执行器。

公式运行示例：

```bash
curl -s http://localhost:8080/api/formula/run \
  -H 'Content-Type: application/json' \
  -d '{"symbol":"000001","script":"T:MA(C,5);","period":"day","right":1,"out_count":3,"calc_count":80}'
```

响应中的 `engine` 表示本次实际使用的执行器；如果 HQChartPy2 执行失败并回退，`fallback_error` 会包含原始错误。

自动化任务 `type` 支持：

| type | 说明 |
| --- | --- |
| `stock_selection` | 股票池 + 公式选股，payload 支持 `formula_id`、`pool_id`、`symbols`、`batch_size`、`continue_on_error` |
| `strategy_selection` | 策略模板选股，策略分数保存在结果详情中 |
| `selection_tracking` | 根据后续交易日 K 线更新 D1/D5/D10 表现，默认目标收益 3%、最大回撤 5% |
| `system_sync` | 系统同步任务，payload 使用 `scope` 控制同步范围 |
| `custom` | 自定义任务，支持 `noop`、`system_sync`、`http_request` |

`system_sync` 常用 payload：

```json
{"scope":"basic"}
```

```json
{"scope":"kline","tables":["day"],"limit":4}
```

```json
{"scope":"all","tables":["day"],"limit":4,"max_codes":200,"with_index":true,"continue_on_error":true}
```

可用 `scope` 包括 `basic`、`codes`、`workday`、`kline`、`gbbq`、`finance`、`f10`、`block`、`industry`、`stat`、`stat2`、`xgsg`、`all`。快照型同步会把 JSON 写入 `data/database/snapshots/`。

`custom` 示例：

```json
{"action":"system_sync","sync":{"scope":"block","block_files":["gn"],"with_index":true}}
```

`/api/hqchart/history` 返回的 `data` 为 HQChart K 线数组：

```text
[date, yclose, open, high, low, close, vol, amount, time]
```

## 维护说明

- API 测试可使用 Postman、cURL 或 `scripts/manual/run_api_checks.py`。
- 旧的阶段性 API 总结和集成说明已合并到本文件，历史去向见 [文档历史](document-history.md)。
