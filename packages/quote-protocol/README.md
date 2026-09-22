# quote-protocol

Go client for the TCP quote protocol.

Implemented protocol base:

- `RT_LOGIN` on `Client.Start`
- `RT_KEEPACTIVE` heartbeat loop
- background read loop
- reconnect after connection errors
- zipped response unwrap via `RT_ZIPDATA`
- all generated `RT_*` function id constants from `hsds_comudata.h`
- generated `CallRT_*` methods for every function id name
- generic `AskData` request by type or function name
- typed `RT_SERVERTIME`
- typed `RT_STOCKINFOLIST`
- typed `RT_REALTIME_EXT_INT64`

Example:

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

For unparsed functions, call `Request` or `RequestFunction` and parse `Response.Body` according to the protocol document.

Every generated function id also has an explicit call method:

```go
resp, err := client.CallRT_TREND_INT64(ctx, quoteprotocol.FunctionCallOptions{
    Size:    1,
    Payload: payload,
})
```

Use `FunctionCapabilities()` to list all function ids and see whether each one is currently `typed`, `raw`, or protocol-level support.
