# Quote Core Service

`quote-core-service` is the debug shell for the TCP quote protocol.

The reusable client lives in `packages/quote-protocol`. The current focus is the TCP protocol core: login, heartbeat, long-connection read loop, reconnect, zipped responses, function id discovery, generic `AskData` requests, server time, and realtime quote parsing. It is not yet wired into `market-service` as an HTTP replacement for TDX.

Default probe target:

```text
222.66.65.68:8888
```

Also tested for `RT_LOGIN`: `115.28.252.213:8888`.

Note: `39.98.157.150:8090` belongs to the JSON push-subscription protocol documented as `A101/B201` messages ending with `$_`, not the binary `RT_LOGIN` probe.

## Run

```bash
cd services/quote-core-service
go run .
```

Default HTTP port: `8084`.

## Probe Login

```bash
cd services/quote-core-service
go run . -probe-login
```

## Function IDs

```bash
go run . -list-functions
```

Function ids are generated from `C++行情SDK文件/hsds_comudata.h`. The output includes `support`:

- `typed`: has a dedicated parser/helper
- `raw`: has generated SDK call methods and returns raw response bytes
- `protocol`: handled by the protocol layer

## Generic TCP Request

Send any logged-in `AskData` request by function name or hex id:

```bash
go run . -request RT_SERVERTIME
go run . -request RT_TESTSRV
go run . -request 0x0902
go run . -request 0902
```

For request payloads:

```bash
go run . -request RT_REALTIME_EXT_INT64 -size 1 -payload-hex '<CodeInfo hex>'
```

## Realtime Quote

```bash
go run . -quote sh600570,sz000001
```

This uses `RT_REALTIME_EXT_INT64` and currently parses core quote fields plus bid/ask level 1-5.

## Stock Info List

```bash
go run . -stock-info
```

This uses `RT_STOCKINFOLIST` and parses the stable `StockInitInfo` fields: code type, code, name, previous close, and five-day volume.

## Heartbeat Test

```bash
go run . -heartbeat-test 5s
```

This starts the SDK long connection with a 2-second heartbeat interval, waits for the requested duration, and prints client stats.

## SDK Usage

```go
client := quoteprotocol.New(quoteprotocol.Config{
    Addr: "222.66.65.68:8888",
})
if err := client.Start(ctx); err != nil {
    return err
}
defer client.Close()

quotes, err := client.GetRealtime(ctx, []string{"sh600570", "sz000001"})
```

For function ids that do not yet have a typed parser, use the generic request path:

```go
resp, err := client.RequestFunction(ctx, "RT_SERVERTIME", 0, 0, nil)
```

All function id names also have generated SDK methods, such as `CallRT_TREND_INT64` and `CallRT_STOCKTICK_INT64`.

## Minimal HTTP

The HTTP surface is intentionally minimal while the TCP layer is being stabilized:

```bash
curl "http://localhost:8084/api/health"
curl "http://localhost:8084/api/probe/login"
```

## Environment

- `PORT`: HTTP listen port, default `8084`
- `QUOTE_CORE_ADDR`: TCP server address, default `222.66.65.68:8888`
- `QUOTE_CORE_USER`: login username, default `anyone`
- `QUOTE_CORE_PASSWORD`: login password, default `anyone`
- `QUOTE_CORE_TIMEOUT`: TCP read/write timeout, default `5s`
