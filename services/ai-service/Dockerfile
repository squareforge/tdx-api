FROM docker.1ms.run/library/golang:1.25-alpine AS builder

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories
WORKDIR /app

ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    PATH="/usr/local/go/bin:${PATH}"

COPY services/ai-service/go.mod services/ai-service/go.sum ./services/ai-service/
RUN (cd services/ai-service && go mod download)
COPY . .
RUN (cd services/ai-service && go build -ldflags="-s -w" -o /app/ai-service .)

FROM docker.1ms.run/library/alpine:latest

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata wget

ENV TZ=Asia/Shanghai \
    PORT=8083 \
    AUTOMATION_SCHEDULER_ENABLED=false \
    AI_SERVICE_TOKEN=

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app
COPY --from=builder /app/ai-service ./ai-service
RUN chown -R appuser:appuser /app

USER appuser
EXPOSE 8083

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8083/api/health || exit 1

CMD ["./ai-service"]
