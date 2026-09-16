# 容器镜像站点下载系统

这是一个简单的容器镜像下载与管理系统，提供网页登录界面和用户管理功能。项目采用 Go 语言编写，使用纯 Go 的 SQLite 驱动 (modernc.org/sqlite) 作为数据存储。

## 特性
- Web 界面登录授权系统
- 基于 SQLite 的轻量级存储
- 简单易用的模板渲染

## 编译与运行

`ash
# 安装依赖
go mod tidy

# 运行
go run main.go

# 编译
go build -o image-downloader
`
