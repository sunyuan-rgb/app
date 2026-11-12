# 多阶段构建：两个服务二进制（主服务 + 模拟阅读服务）
FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0
# 构建期模块代理可按需覆盖（如国内环境 docker compose build --build-arg GOPROXY=https://goproxy.cn,direct）
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && go build -trimpath -ldflags="-s -w" -o /out/readingsvc ./cmd/readingsvc

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/server /app/server
COPY --from=build /out/readingsvc /app/readingsvc
EXPOSE 8080 8081
# 默认启动主服务；compose 中按服务覆盖 command
CMD ["/app/server"]
