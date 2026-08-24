# ArtifactRegistry

ArtifactRegistry 是一个自建的 OCI 制品 / 容器镜像仓库演示服务，提供内容寻址的
blob 存储、manifest 清单、tag 引用、分块上传断点续传与垃圾回收。

## 功能

- 仓库命名空间：创建、列出、删除仓库。
- 内容寻址存储：层按 `sha256:<hex>` 摘要去重存储，引用计数精确跟踪。
- Manifest：解析、校验 OCI 风格清单，提交前确认引用的层已完整上传。
- Tag：可变标签指向 manifest，拉取按标签解析。
- 分块上传：支持任意顺序分块到达、断点续传与合并校验。
- 垃圾回收：按 tag → manifest → blob 的引用链标记，回收孤儿对象。
- 访问令牌：签发与续期，上传会话绑定令牌族。
- Web 浏览页：`/` 提供仓库浏览页面，`/healthz` 提供健康探测。

## 构建

依赖已 vendor 进 `vendor/`，构建时无需联网：

```sh
go build -mod=vendor ./...
go test -mod=vendor ./...
go vet -mod=vendor ./...
```

## 启动

```sh
go run -mod=vendor ./cmd/artifactregistry -addr :8377
```

启动后：

- `GET /healthz` 返回服务状态。
- `GET /` 返回仓库浏览页面。
- `POST /v1/repos` 创建仓库；`POST /v1/token` 获取推送令牌。

## 镜像

```sh
docker build -t artifactregistry:latest -f benzhi.Dockerfile .
docker run --rm -p 8377:8377 artifactregistry:latest
```

镜像基于 `golang:1.23.12` 构建、`alpine:3.20` 运行，`GOPROXY=off` 且使用
vendor 目录离线构建；运行镜像仅暴露 HTTP 服务与浏览页。
