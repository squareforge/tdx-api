FROM docker.1ms.run/library/golang:1.25-alpine AS builder

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
WORKDIR /app

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    PATH="/usr/local/go/bin:${PATH}"

COPY packages/tdx-core/go.mod packages/tdx-core/go.sum ./packages/tdx-core/
COPY services/market-service/go.mod services/market-service/go.sum ./services/market-service/
RUN (cd packages/tdx-core && go mod download) && (cd services/market-service && go mod download)
COPY . .
RUN (cd services/market-service && go build -ldflags="-s -w" -o /app/market-service .)

FROM docker.1ms.run/library/alpine:latest

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata wget

ENV TZ=Asia/Shanghai \
    PORT=8081

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app
COPY --from=builder /app/market-service ./market-service
RUN chown -R appuser:appuser /app

USER appuser
EXPOSE 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/health || exit 1

CMD ["./market-service"]
