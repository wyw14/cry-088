# syntax=docker/dockerfile:1.7
# 评测用镜像：保留完整 Go 工具链，依赖构建期预下载。
FROM golang:1.25
WORKDIR /app
COPY go.mod go.sum ./
RUN --mount=type=cache,id=go-annotation-gomod,target=/go/pkg/mod,sharing=locked go mod download
COPY . .
RUN --mount=type=cache,id=go-annotation-gomod,target=/go/pkg/mod,sharing=locked --mount=type=cache,id=go-annotation-build,target=/root/.cache/go-build,sharing=locked go build ./...
CMD ["bash"]

# 多架构交叉构建示例（如需交付双架构镜像）：
# docker buildx build --platform linux/arm64,linux/amd64 -f benzhi.Dockerfile -t <image> .
