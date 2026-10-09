# 部署与运行

## Docker

推荐使用 Docker Compose：

```bash
docker compose up -d --build
docker compose logs -f
```

访问 `http://localhost:8080`。

当前发布版本为 `0.1.3-beta.1`，可通过 `GET /api/server-status` 查看 Web 服务版本；版本来源记录在仓库根目录 `VERSION`。

当前 Docker 是多服务形态：Web、行情服务、公式引擎和指标选股服务分别运行在独立容器里。Web 对外暴露 `8080`，其他服务也保留宿主机调试端口。

`docker-compose.yml` 会把宿主机 `./data` 挂载到容器 `/app/data`，自动化数据库、K 线库、同步快照和任务结果都会保留在项目目录下。重建镜像不会清空这些数据。

常用维护命令：

```bash
docker compose restart stock-web
docker compose restart market-service
docker compose restart formula-worker
docker compose restart selection-worker
docker compose down
docker compose down --rmi all
```

只重建某个服务：

```bash
docker compose up -d --build stock-web
docker compose up -d --build market-service
docker compose up -d --build formula-worker
docker compose up -d --build selection-worker
```

如果 Docker Hub 拉取基础镜像超时，在 Docker Desktop 的 Docker Engine 配置里添加 registry mirrors，或在网络条件更好的环境构建。

## 源码运行

要求 Go 1.25+。

```bash
python3 services/formula-worker/worker.py
cd services/market-service
go run .
cd ../selection-worker
go run .
cd ../../services/ai-service
go run .
cd ../../apps/web
go run .
```

如果 `8080` 被占用，可临时指定端口：

```bash
cd apps/web
PORT=18080 go run .
```

Windows 可运行 `apps/web/start.bat`，macOS/Linux 可运行：

```bash
cd apps/web
chmod +x start.sh
./start.sh
```

## 验证

```bash
curl "http://localhost:8080/api/health"
curl "http://localhost:8080/api/quote?code=000001"
curl "http://localhost:8080/api/formula/health"
```

核心模块和 Web 子模块测试：

```bash
cd packages/tdx-core
GOPROXY=https://goproxy.cn,direct go test ./...

cd ../workbench-core
GOPROXY=https://goproxy.cn,direct go test ./...

cd ../../apps/web
GOPROXY=https://goproxy.cn,direct go test ./...
GOPROXY=https://goproxy.cn,direct go build -o /tmp/tdx-workbench-web .

cd ../../services/market-service
GOPROXY=https://goproxy.cn,direct go test ./...
GOPROXY=https://goproxy.cn,direct go build -o /tmp/tdx-market-service .

cd ../selection-worker
GOPROXY=https://goproxy.cn,direct go test ./...
GOPROXY=https://goproxy.cn,direct go build -o /tmp/tdx-selection-worker .

cd ../ai-service
GOPROXY=https://goproxy.cn,direct go test ./...
```

## 常见问题

| 问题 | 处理 |
| --- | --- |
| `go` 命令找不到 | 安装 Go 1.25+，确认 `go version` 正常 |
| 模块下载慢 | 设置 `GOPROXY=https://goproxy.cn,direct` |
| 端口 8080 被占用 | 源码运行设置 `PORT=18080`，Docker 调整端口映射 |
| 公式测试失败 | Docker 看容器日志；源码运行确认 `python3 services/formula-worker/worker.py` 正在运行 |
| Web 启动失败 | 用 `go run .`，不要只运行 `server.go` |
| Docker 构建失败 | 先看 `docker compose logs -f`，再确认基础镜像能拉取 |
| 自动化数据丢失 | 确认 Compose 中存在 `./data:/app/data` 挂载，不要手动删除项目下的 `data/` |
