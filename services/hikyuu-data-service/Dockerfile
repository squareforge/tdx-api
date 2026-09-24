FROM docker.1ms.run/library/python:3.11-bookworm

ENV DEBIAN_FRONTEND=noninteractive \
    TZ=Asia/Shanghai \
    PORT=8091 \
    HIKYUU_STOCKS_DIR=/root/stocks \
    HIKYUU_CONFIG_DIR=/root/.hikyuu \
    HIKYUU_LOG_DIR=/app/logs \
    HIKYUU_QUERY_SCRIPT=/app/query_runner.py \
    HIKYUU_QUERY_TIMEOUT_SECONDS=120 \
    HIKYUU_MAX_CONCURRENT_QUERIES=4 \
    HIKYUU_SCHEDULER_ENABLED=true \
    HIKYUU_AFTER_CLOSE_CRON="0 17 * * 1-5" \
    HIKYUU_IMPORT_MAX_ATTEMPTS=3 \
    HIKYUU_IMPORT_RETRY_DELAY_SECONDS=5

WORKDIR /app

# Use a domestic Debian mirror to keep ARM64 image builds reliable in China.
RUN sed -i \
        -e 's|deb.debian.org/debian-security|mirrors.aliyun.com/debian-security|g' \
        -e 's|deb.debian.org/debian|mirrors.aliyun.com/debian|g' \
        /etc/apt/sources.list.d/debian.sources && \
    apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata wget libgomp1 libgl1 libglib2.0-0 && \
    rm -rf /var/lib/apt/lists/*

COPY services/hikyuu-data-service/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple --trusted-host pypi.tuna.tsinghua.edu.cn -r requirements.txt && \
    pip install --no-cache-dir -i https://pypi.tuna.tsinghua.edu.cn/simple --trusted-host pypi.tuna.tsinghua.edu.cn hikyuu==2.8.2

COPY services/hikyuu-data-service/app.py ./app.py
COPY services/hikyuu-data-service/importdata_runner.py ./importdata_runner.py
COPY services/hikyuu-data-service/freshness_runner.py ./freshness_runner.py
COPY services/hikyuu-data-service/query_runner.py ./query_runner.py
COPY services/hikyuu-data-service/research_runner.py ./research_runner.py

RUN mkdir -p /root/stocks /root/.hikyuu /app/logs

EXPOSE 8091

HEALTHCHECK --interval=30s --timeout=3s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 -O /dev/null http://localhost:8091/api/hikyuu/health || exit 1

CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8091"]
