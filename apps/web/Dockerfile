FROM docker.1ms.run/library/golang:1.25-alpine AS builder

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
WORKDIR /app

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    PATH="/usr/local/go/bin:${PATH}"

COPY packages/tdx-core/go.mod packages/tdx-core/go.sum ./packages/tdx-core/
COPY packages/workbench-core/go.mod packages/workbench-core/go.sum ./packages/workbench-core/
COPY apps/web/go.mod apps/web/go.sum ./apps/web/
RUN (cd packages/tdx-core && go mod download) && (cd packages/workbench-core && go mod download) && (cd apps/web && go mod download)
COPY . .
RUN (cd apps/web && go build -ldflags="-s -w" -o /app/stock-web .)

FROM docker.1ms.run/library/alpine:latest

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata wget python3 py3-pip && \
    PIP_BREAK_SYSTEM_PACKAGES=1 python3 -m pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple --trusted-host pypi.tuna.tsinghua.edu.cn pandas_market_calendars

ENV TZ=Asia/Shanghai \
    TDX_APP_ROLE=web \
    AUTOMATION_SCHEDULER_ENABLED=false

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app
COPY --from=builder /app/stock-web ./stock-web
COPY apps/web/static ./static
COPY apps/web/static-react ./static-react
RUN chown -R appuser:appuser /app

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

CMD ["./stock-web"]
