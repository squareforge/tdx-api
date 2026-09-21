# Quote Core Service

`quote-core-service` is the experimental TCP quote gateway for the Investment Winner / H5-style market protocol.

The first version only performs a minimal `RT_LOGIN` probe against a configured quote server. It does not yet serve realtime quotes to `market-service`.

Default probe target:

```text
39.98.157.150:8090
```

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

Or through HTTP:

```bash
curl "http://localhost:8084/api/probe/login"
```

## Environment

- `PORT`: HTTP listen port, default `8084`
- `QUOTE_CORE_ADDR`: TCP server address, default `39.98.157.150:8090`
- `QUOTE_CORE_USER`: login username, default `anyone`
- `QUOTE_CORE_PASSWORD`: login password, default `anyone`
- `QUOTE_CORE_TIMEOUT`: TCP read/write timeout, default `5s`
