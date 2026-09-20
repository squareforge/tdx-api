#!/bin/bash
#
# TDX Workbench Docker 部署脚本
# 用途: 一键启动所有服务
#

set -e

echo "╔══════════════════════════════════════════════════════════════════════════╗"
echo "║                   TDX Workbench - Docker 部署脚本                         ║"
echo "╚══════════════════════════════════════════════════════════════════════════╝"
echo ""

# 检查Docker环境
echo "🔍 检查Docker环境..."
if ! command -v docker &> /dev/null; then
    echo "❌ Docker未安装，请先安装Docker Desktop"
    exit 1
fi

if ! docker compose version &> /dev/null; then
    echo "❌ Docker Compose未安装或版本过低，请安装Docker Compose v2+"
    exit 1
fi

echo "✓ Docker环境检查通过"
echo ""

# 检查是否在项目根目录
if [ ! -f "docker-compose.yml" ]; then
    echo "❌ 请在项目根目录执行此脚本"
    exit 1
fi

echo "✓ 项目目录检查通过"
echo ""

# 创建必要的数据目录
echo "📁 创建数据目录..."
mkdir -p data/database
mkdir -p data/hikyuu/stocks
mkdir -p data/hikyuu/config
mkdir -p data/hikyuu/logs
echo "✓ 数据目录创建完成"
echo ""

# 停止旧容器（如果存在）
echo "🛑 停止旧容器（如果存在）..."
docker compose down 2>/dev/null || true
echo "✓ 旧容器已停止"
echo ""

# 构建并启动服务
echo "🚀 开始构建并启动服务..."
echo "   这可能需要几分钟时间，请耐心等待..."
echo ""

docker compose up -d --build

echo ""
echo "⏳ 等待服务启动..."
sleep 10

# 检查服务状态
echo ""
echo "📊 服务状态："
echo "─────────────────────────────────────────────────────────────────────────"
docker compose ps

echo ""
echo "╔══════════════════════════════════════════════════════════════════════════╗"
echo "║                           ✅ 部署完成                                     ║"
echo "╚══════════════════════════════════════════════════════════════════════════╝"
echo ""
echo "🌐 Web工作台: http://localhost:8080"
echo ""
echo "📡 各服务端口："
echo "   • Web工作台:        8080"
echo "   • 市场行情服务:     18081"
echo "   • 公式服务:         18712"
echo "   • Hikyuu数据服务:   18091"
echo "   • 选股服务:         18082"
echo "   • AI服务:           18083"
echo ""
echo "🔍 检查服务健康状态："
echo "   curl http://localhost:8080/api/health"
echo "   curl http://localhost:8080/api/services/status"
echo ""
echo "📋 常用命令："
echo "   docker compose logs -f              # 查看所有日志"
echo "   docker compose logs -f stock-web    # 查看Web服务日志"
echo "   docker compose restart stock-web    # 重启Web服务"
echo "   docker compose down                 # 停止所有服务"
echo ""
echo "💡 首次使用："
echo "   1. 打开 http://localhost:8080"
echo "   2. 进入 AI模型 工作区配置AI密钥（可选）"
echo "   3. 进入 自选 或 市场行情 查看数据"
echo ""
echo "⚠️  注意："
echo "   • 首次启动数据同步可能需要几分钟"
echo "   • AI功能需要配置API密钥才能使用"
echo "   • 行情数据可能延迟或缺失，仅供学习研究"
echo ""
