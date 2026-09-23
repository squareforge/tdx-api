# 多阶段构建 - 第一阶段：构建
# 使用官方镜像（如果国内拉取慢，可以配置docker daemon的registry-mirrors）
FROM docker.1ms.run/library/golang:1.25-alpine AS builder

# 替换Alpine镜像源为阿里云
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

# 设置工作目录
WORKDIR /app

# 设置Go代理（使用国内镜像加速）
ENV GO111MODULE=on \
    GOPROXY=https://goproxy.cn,https://mirrors.aliyun.com/goproxy/,direct \
    GOTOOLCHAIN=local \
    CGO_ENABLED=0 \
    PATH="/usr/local/go/bin:${PATH}"

# 复制 Go 模块文件
COPY packages/tdx-core/go.mod packages/tdx-core/go.sum ./packages/tdx-core/
COPY packages/workbench-core/go.mod packages/workbench-core/go.sum ./packages/workbench-core/
COPY apps/web/go.mod apps/web/go.sum ./apps/web/
RUN (cd packages/tdx-core && go mod download) && (cd packages/workbench-core && go mod download) && (cd apps/web && go mod download)

# 复制整个项目的源代码
COPY . .

# 在子 shell 中编译，避免模块路径混淆问题
RUN (cd apps/web && go build -ldflags="-s -w" -o /app/stock-web .)

# 多阶段构建 - 第二阶段：运行
FROM docker.1ms.run/library/alpine:latest

# 替换Alpine镜像源为阿里云，安装必要的运行时依赖
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk --no-cache add ca-certificates tzdata wget python3 py3-pip libstdc++ && \
    PIP_BREAK_SYSTEM_PACKAGES=1 python3 -m pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple --trusted-host pypi.tuna.tsinghua.edu.cn pandas_market_calendars

# 设置时区为上海
ENV TZ=Asia/Shanghai

# 创建非root用户
RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

# 设置工作目录
WORKDIR /app

COPY services/formula-worker ./formula-worker

RUN apk --no-cache add --virtual .hqchart-build g++ python3-dev zlib-dev linux-headers && \
    python3 /app/formula-worker/install_hqchartpy2.py && \
    apk del .hqchart-build

COPY --from=builder /app/stock-web .
COPY apps/web/static ./static
COPY deploy/docker-entrypoint.sh ./docker-entrypoint.sh

# 更改文件所有者
RUN chmod +x /app/docker-entrypoint.sh /app/formula-worker/worker.py && \
    chown -R appuser:appuser /app

# 切换到非root用户
USER appuser

# 暴露端口
EXPOSE 8080

# 健康检查
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

# 启动应用
CMD ["./docker-entrypoint.sh"]
