# TDX Workbench - Docker 部署指南

## ✅ 当前状态

你的TDX Workbench已经成功部署并运行中！

### 运行中的服务

| 服务名称 | 状态 | 端口 | 说明 |
|---------|------|------|------|
| tdx-workbench-web | ✅ Healthy | 8080 | Web工作台（主界面） |
| tdx-workbench-market | ✅ Healthy | 18081 | 市场行情服务 |
| tdx-workbench-formula | ✅ Healthy | 18712 | 公式计算服务 |
| tdx-workbench-selection | ✅ Healthy | 18082 | 选股服务 |
| tdx-workbench-ai | ✅ Healthy | 18083 | AI分析服务 |

**注意：hikyuu-data-service 未运行，但不影响基本功能。**

---

## 🌐 访问地址

### Web工作台（推荐）
```
http://localhost:8080
```

### API端点
- Web API: http://localhost:8080/api/health
- 服务状态: http://localhost:8080/api/services/status
- 市场行情: http://localhost:18081/api/health
- AI服务: http://localhost:18083/api/health

---

## 📋 常用命令

### 查看服务状态
```bash
docker compose ps
```

### 查看日志
```bash
# 所有服务日志
docker compose logs -f

# 单个服务日志
docker compose logs -f stock-web       # Web服务
docker compose logs -f market-service  # 行情服务
docker compose logs -f ai-service      # AI服务
docker compose logs -f selection-worker # 选股服务

# 最近100行
docker compose logs --tail=100 stock-web
```

### 重启服务
```bash
# 重启所有服务
docker compose restart

# 重启单个服务
docker compose restart stock-web
docker compose restart market-service

# 重新构建并重启
docker compose up -d --build stock-web
```

### 停止和启动
```bash
# 停止所有服务
docker compose down

# 启动所有服务
docker compose up -d

# 停止并删除数据卷（⚠️ 会删除数据）
docker compose down -v
```

### 进入容器调试
```bash
# 进入Web容器
docker exec -it tdx-workbench-web sh

# 进入市场服务容器
docker exec -it tdx-workbench-market sh

# 查看容器内文件
docker exec tdx-workbench-web ls -la /app
```

---

## 🔧 启动 Hikyuu 数据服务（可选）

Hikyuu 提供历史K线数据，如果需要使用回测等高级功能，可以启动它：

```bash
# 创建Hikyuu数据目录
mkdir -p data/hikyuu/stocks
mkdir -p data/hikyuu/config
mkdir -p data/hikyuu/logs

# 启动Hikyuu服务
docker compose up -d hikyuu-data-service

# 查看启动日志
docker compose logs -f hikyuu-data-service
```

首次启动Hikyuu需要下载历史数据，可能需要较长时间（15-30分钟）。

---

## 🎯 使用指南

### 1. 首次访问

打开浏览器访问：
```
http://localhost:8080
```

你会看到以下工作区：
- 自选：查看自选股票实时行情
- 市场行情：查看K线图和分时图
- 数据中心：查询股票信息
- 选股结果：查看策略选股结果
- 策略中心：管理和运行策略
- AI模型：配置AI服务

### 2. 应用你的修正策略

现在可以应用之前生成的修正策略配置：

1. 打开 **策略中心** → **新建策略**
2. 复制 `output/fixed_strategy_v3.json` 的 `config` 部分
3. 保存为 "修正版V3-回踩突破策略"
4. 点击 **执行策略**

### 3. 配置AI服务（可选）

如果要使用AI分析功能：

1. 进入 **AI模型** 工作区
2. 添加API配置：
   - DeepSeek（推荐）
   - OpenAI
   - 通义千问
   - 智谱GLM

可以设置环境变量：
```bash
# 停止服务
docker compose down

# 编辑 .env 文件
cat > .env << EOF
DEEPSEEK_API_KEY=your_deepseek_api_key
AI_SERVICE_TOKEN=optional_token
EOF

# 重启服务
docker compose up -d
```

---

## 📊 数据说明

### 数据目录结构
```
data/
├── database/          # SQLite数据库
│   └── workbench.db  # 主数据库
├── hikyuu/           # Hikyuu历史数据（可选）
│   ├── stocks/       # 股票数据
│   ├── config/       # 配置文件
│   └── logs/         # 日志
└── market/           # 市场数据缓存
```

### 数据同步

市场服务会自动同步：
- 股票列表
- 实时行情
- 分时数据
- K线数据

首次启动可能需要几分钟进行数据初始化。

---

## ⚠️ 常见问题

### Q1: 服务启动失败

```bash
# 查看错误日志
docker compose logs

# 检查端口占用
lsof -i :8080
lsof -i :18081

# 清理并重新启动
docker compose down
docker compose up -d
```

### Q2: 无法访问Web界面

1. 确认服务状态：`docker compose ps`
2. 检查健康检查：`curl http://localhost:8080/api/health`
3. 查看日志：`docker compose logs -f stock-web`
4. 确认没有防火墙阻止

### Q3: 数据不更新

```bash
# 重启市场服务
docker compose restart market-service

# 查看同步日志
docker compose logs -f market-service

# 手动触发同步
curl -X POST http://localhost:18081/api/sync/all
```

### Q4: 内存不足

编辑 `docker-compose.yml` 添加资源限制：

```yaml
services:
  stock-web:
    # ... 其他配置
    deploy:
      resources:
        limits:
          memory: 1G
        reservations:
          memory: 512M
```

### Q5: Hikyuu数据下载慢

Hikyuu首次下载历史数据可能很慢：
- 耐心等待15-30分钟
- 查看日志：`docker compose logs -f hikyuu-data-service`
- 如果卡住，可以重启服务

---

## 🔐 安全建议

### 生产环境部署

当前配置适合**本地试用**，如果要在服务器部署：

1. **修改默认端口**
   ```yaml
   ports:
     - "8080:8080"  # 改为内部端口
   ```

2. **配置AI_SERVICE_TOKEN**
   ```bash
   export AI_SERVICE_TOKEN="your-strong-token"
   ```

3. **使用反向代理**
   ```bash
   # Nginx配置示例
   server {
       listen 80;
       server_name tdx.example.com;
       location / {
           proxy_pass http://localhost:8080;
       }
   }
   ```

4. **启用HTTPS**
5. **限制访问IP**
6. **定期备份数据库**

---

## 📈 性能优化

### 调整资源限制

编辑 `docker-compose.yml`:

```yaml
services:
  stock-web:
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
```

### 数据库优化

```bash
# 进入容器
docker exec -it tdx-workbench-web sh

# 优化数据库
sqlite3 /app/data/database/workbench.db "VACUUM;"
sqlite3 /app/data/database/workbench.db "ANALYZE;"
```

---

## 🔄 更新和升级

### 拉取最新代码

```bash
# 停止服务
docker compose down

# 拉取最新代码
git pull

# 重新构建并启动
docker compose up -d --build

# 备份数据（推荐）
cp -r data data.backup.$(date +%Y%m%d)
```

### 数据迁移

```bash
# 导出策略配置
sqlite3 data/database/workbench.db "SELECT * FROM strategies;" > strategies_backup.sql

# 导入到新数据库
sqlite3 new_database.db < strategies_backup.sql
```

---

## 📞 获取帮助

### 日志位置
```bash
# Docker日志
docker compose logs

# Hikyuu日志
cat data/hikyuu/logs/*.log

# 应用日志
docker exec tdx-workbench-web cat /app/logs/app.log
```

### 健康检查
```bash
curl http://localhost:8080/api/health
curl http://localhost:8080/api/services/status
```

### 重置服务
```bash
# ⚠️ 这会删除所有数据！
docker compose down -v
rm -rf data/
docker compose up -d --build
```

---

## 🎉 下一步

你的TDX Workbench已经就绪！现在可以：

1. ✅ 访问 http://localhost:8080
2. ✅ 应用修正后的策略配置（`output/fixed_strategy_v3.json`）
3. ✅ 运行选股并追踪3-5天
4. ✅ 验证排名改进效果

**祝你使用愉快！** 🚀

---

*更新时间: 2026-09-20*
*部署方式: Docker Compose*
*版本: 0.1.0*
