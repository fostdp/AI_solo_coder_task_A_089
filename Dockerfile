# syntax=docker/dockerfile:1.6

# ==================== Build Stage ====================
FROM golang:1.21-alpine3.19 AS builder

WORKDIR /app

# 安装构建依赖
RUN apk add --no-cache git ca-certificates tzdata

# 设置Go环境变量 - 静态编译
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GIN_MODE=release

# -------------------- Build Backend --------------------
WORKDIR /app/backend

# 复制go mod和sum
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 复制源码
COPY backend/ .

# 构建静态二进制
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -tags=netgo,osusergo \
    -o /app/bin/plankroad-backend .

# -------------------- Build Simulator --------------------
WORKDIR /app/simulator

COPY simulator/go.mod simulator/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY simulator/ .

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build \
    -ldflags="-s -w -extldflags '-static'" \
    -tags=netgo,osusergo \
    -o /app/bin/plankroad-simulator .

# ==================== Runtime Stage ====================
FROM alpine:3.19 AS runtime

WORKDIR /app

# 安装运行时依赖
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    && rm -rf /var/cache/apk/*

# 设置时区
ENV TZ=Asia/Shanghai

# 创建非root用户
RUN addgroup -g 10001 -S plankroad \
    && adduser -u 10001 -S plankroad -G plankroad

# 复制二进制和配置
COPY --from=builder /app/bin/plankroad-backend /app/bin/
COPY --from=builder /app/bin/plankroad-simulator /app/bin/
COPY --from=builder /app/backend/config/params /app/config/params/
COPY --from=builder /app/backend/static /app/static/
COPY --from=builder /app/backend/.env /app/.env
COPY db/init.sql /app/db/

# 设置权限
RUN chown -R plankroad:plankroad /app \
    && chmod +x /app/bin/plankroad-backend \
    && chmod +x /app/bin/plankroad-simulator

USER plankroad

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD curl -f http://localhost:${SERVER_PORT:-8080}/api/sites || exit 1

EXPOSE 8080
EXPOSE 6060
EXPOSE 9090

CMD ["/app/bin/plankroad-backend"]
