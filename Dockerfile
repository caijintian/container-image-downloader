FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 预加载依赖
COPY go.mod go.sum ./
RUN go mod download

# 复制源码并编译
COPY . .
# 禁用 CGO，减小体积并实现跨平台运行，去除调试信息
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o image-downloader main.go

# 运行环境使用极其轻量的 alpine
FROM alpine:latest

WORKDIR /app
# 从编译阶段复制编译好的二进制文件
COPY --from=builder /app/image-downloader .

# 暴露端口
EXPOSE 8989

# 启动程序
CMD ["./image-downloader"]
