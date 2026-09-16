# Container Image Downloader (容器镜像站点下载系统)

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go)
![SQLite](https://img.shields.io/badge/Database-SQLite3-003B57?style=flat-square&logo=sqlite)
![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)

基于 Go + SQLite 开发的极简私有镜像下载站点，附带 Web 界面和用户登录授权。

开发初衷是为了在内部团队快速分发镜像，同时对访问权限做一点基础的管控。项目没有借助任何重量级框架，前端页面直接内嵌在 Go 二进制程序里，所以部署起来只有一个单文件，数据也都存在本地 db 里，维护起来非常省事。

## 界面截图

前端登录页面：

![登录页面](./screenshot.png)

后台管理控制台：

![后台管理](./dashboard.png)

## 主要特性

- 纯 Go 实现（使用了 modernc.org/sqlite），没有 CGO 依赖，交叉编译非常方便。
- 极简部署：无复杂配置文件，编译产物仅一个可执行文件，数据全部存在同目录下的 fetcher.db 中。
- 内置基于 Token 的登录会话机制。
- 支持 Docker 容器化一键部署。

## 快速开始

克隆代码并拉取依赖：
```bash
git clone https://github.com/caijintian/container-image-downloader.git
cd container-image-downloader
go mod tidy
```

本地运行：
```bash
go run main.go
# 默认监听端口 8989
```

生产编译：
```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o image-downloader main.go
```

## 目录结构
```text
.
├── main.go        # 业务逻辑与内嵌前端 HTML
├── Dockerfile     
└── go.mod         
```

## 协议
MIT License
