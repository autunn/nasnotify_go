# 核心秘诀：强制使用宿主机(x86)原生环境作为 builder，绕过极慢的 QEMU 模拟器
FROM --platform=$BUILDPLATFORM golang:alpine AS builder

# 接收 GitHub Actions 传进来的版本号
ARG APP_VERSION=v2026.05.01
# 核心秘诀：接收 Docker 自动传进来的目标架构 (例如 amd64, arm64, arm)
ARG TARGETARCH

WORKDIR /app
RUN apk add --no-cache git

# 复制整个项目（包含 go.mod, cmd, internal, frontend 等）
COPY . .

RUN go mod download

# 核心修改：将编译目标路径从 . 改为 ./cmd/nasnotify
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X main.Version=${APP_VERSION}" \
    -o nasnotify-go-app ./cmd/nasnotify

# 构建 Vite 前端，最终镜像会把 dist 放到 /app/www
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /app/frontend/ugreen-app
COPY frontend/ugreen-app/package*.json ./
RUN npm ci
COPY frontend/ugreen-app/ ./
RUN npm run build

# ==========================================
# 最终运行阶段：拉取对应架构的 alpine 基础镜像
FROM alpine:latest
WORKDIR /app

# 配置时区为上海
RUN apk add --no-cache ca-certificates tzdata \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

# 把极速编译好的二进制文件复制过来
COPY --from=builder /app/nasnotify-go-app .
COPY --from=frontend /app/frontend/ugreen-app/dist ./www
EXPOSE 5080
VOLUME ["/app/data", "/app/config"]
CMD ["./nasnotify-go-app"]
