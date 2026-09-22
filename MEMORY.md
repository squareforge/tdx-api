# Project Memory

## Project Overview

- TDX Workbench is a local A-share market-data and research workspace.
- Go services expose TDX market data; the React frontend consumes the Web gateway APIs.

## Tech Stack

- Go 1.25 (the Docker builders use the 1.25 image directly with `GOTOOLCHAIN=local`)
- React 19, TypeScript and Vite
- Docker Compose for the multi-service local deployment

## Architecture

- `apps/web`: Web UI and API gateway
- `services/market-service`: market-data API
- `services/quote-core-service`: experimental external binary TCP quote gateway
- `services/hikyuu-data-service`: full and after-close historical-data ingestion backed by HDF5
- `services/selection-worker`: scheduled selection, shared K-line loading and historical strategy replay
- `packages/tdx-core`: TDX protocol client and data models

## Conventions

- `protocol.Price` values use milli-yuan units and must be divided by 1000 for display.
- Quote `Kline.Volume` uses lots and must be multiplied by 100 to display shares.

## Decisions

- Frontend quote consumers normalize the current `Kline` response and the legacy `K` response through shared helpers in `apps/web/frontend/src/lib/format.ts`.
- Selection tracking treats signals created before 09:30 Asia/Shanghai as executable at the signal-day open and counts that session as D1. Later signals start their observation window on the next trading session.
- Market data consumers use `market-service` as the only K-line entry point. It reads Hikyuu local history first and falls back to TDX when a symbol is absent; selection and historical backtests must not read legacy per-symbol SQLite files directly.
- Hikyuu history is requested with forward adjustment. The rare TDX fallback returns raw daily bars and must not be merged into the same series; a later Hikyuu sync remains the authoritative repair.
- Hikyuu owns full initialization and after-close incremental persistence. The fixed close task runs at 16:30 on weekdays, verifies the exchange calendar, waits for Hikyuu to finish, and only then triggers the system-strategy daily batch. Its timeout is two hours.
- Compose disables Hikyuu's internal scheduler because `selection-worker` owns the close-sync workflow; this avoids duplicate HDF5 writers.
- `market-service` owns one shared standard TDX client. `tdx.Manage` receives a one-slot pool backed by that client, so reconnect/failover is coordinated by one `DialDefault` connection instead of several independent scans.
- Fixed D1/D5/D10 tracking runs at 17:15.
- Quote core service starts as a sidecar probe for the external binary TCP quote protocol. Its first milestone is `RT_LOGIN` probing against `222.66.65.68:8888` and `115.28.252.213:8888`; it is not yet wired into `market-service` fallback. `39.98.157.150:8090` is the separate JSON push-subscription protocol.
- External quote symbols reuse the existing `codes` table instead of introducing a separate code table. `CodeModel` carries `CodeType`, `CodeTypeHex`, `CodeTypeSource`, and `CodeTypeUpdatedAt`; initial values are inferred by `tdx_rule`, and later `RT_STOCKINFOLIST` can promote authoritative values with source `stock_info_list`.

## Pitfalls

- `/api/quote` serializes the Go field as `Kline`, with OHLC, volume, and amount nested inside it. Do not read only legacy `K`, `TotalHand`, or top-level `Amount` fields.
- Quote rate is encoded as a signed 16-bit hundredth-percent value. Decode through `int16`; treating it as unsigned turns small negative values into `655.xx%`.
- Tracking calculations must ignore placeholder or malformed daily bars: date and OHLC must be positive, and high/low must contain open and close.
- `000001` is ambiguous: `AddPrefix` maps the bare code to `sz000001` (平安银行) because `isSZStock` (leading `0`) is checked before `isSHIndex` (leading `000`). Reach 上证指数 only via the explicit `sh000001` prefix. Index K-lines must go through the index API (`KindIndex`) — the stock path (`KindStock`) misparses the 4 extra up/down-count bytes per bar.
- Legacy files under `data/database/kline/*.db` and the newer `data/database/kline/day-kline/*.db` have different layouts and update histories. They are retained for compatibility but are not authoritative for strategy history.
- Hikyuu's importer may exit with code 0 after emitting `HDF5_IMPORT/THREAD/FAILURE`; task success must inspect that failure event before publishing a new `data_revision` or starting strategies.
- Hikyuu reads run in short-lived subprocesses and are serialized inside `hikyuu-data-service`; an active sync rejects local reads so `market-service` can fall back to TDX. This prevents the Hikyuu runtime from retaining HDF5 read locks across the after-close writer.
