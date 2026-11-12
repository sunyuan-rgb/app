.PHONY: build run run-reading test cover vet fmt clean db-migrate

# 本地敏感配置从 .env / .env.test 读取（均不入库，见 .env.example）
-include .env
-include .env.test

MYSQL_ROOT_PASSWORD ?=
DB_CONTAINER ?= dev-mysql

build:
	go build ./...

# 主服务（可用 TEACH_CLOCK_OFFSET=24h 启用教学时钟）
run:
	go run ./cmd/server

# 模拟阅读服务（默认 :8081，投递到 MAIN_SVC_URL）
run-reading:
	go run ./cmd/readingsvc

# 依序执行全部迁移（容器内 MySQL）；口令仅从环境变量注入
db-migrate:
	@test -n "$(MYSQL_ROOT_PASSWORD)" || (echo "MYSQL_ROOT_PASSWORD 未设置，请复制 .env.example 为 .env 并填写" >&2; exit 1)
	docker cp migrations $(DB_CONTAINER):/tmp/migrations
	docker exec $(DB_CONTAINER) sh -c "for f in /tmp/migrations/*.sql; do sed -i '1s/^\\xEF\\xBB\\xBF//' $$f; mysql --default-character-set=utf8mb4 -uroot -p'$(MYSQL_ROOT_PASSWORD)' reading_activity < $$f; done"

# 测试库连接串由 TEST_DSN 注入（见 .env.example），仓库内不含任何口令
# -p 2：Windows 上 -race 并行链接 6 个包会耗尽页面文件（fork/exec link.exe: paging file is too small），
# 限制到 2 个并发构建即可稳定；测试数据本身已按包隔离，并行执行不再互相干扰。
test:
	@test -n "$(TEST_DSN)" || (echo "TEST_DSN 未设置，请复制 .env.example 为 .env.test 并填写" >&2; exit 1)
	go test -race -p 2 ./...

cover:
	@test -n "$(TEST_DSN)" || (echo "TEST_DSN 未设置，请复制 .env.example 为 .env.test 并填写" >&2; exit 1)
	go test -race -p 2 -covermode=atomic -coverpkg=./... -coverprofile=coverage.out ./...
	go run ./scripts/covergate coverage.out
	go tool cover -func=coverage.out

vet:
	go vet ./...

fmt:
	gofmt -l -w .

clean:
	go clean
