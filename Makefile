.PHONY: dev infra test migrate-up migrate-down sqlc lint check-ffmpeg check-migrations

dev:
	docker compose up --build

infra:
	docker compose up postgres redis

test:
	@echo "TODO: run tests"

migrate-up:
	goose -dir ./migrations postgres "$$DATABASE_URL" up

migrate-down:
	goose -dir ./migrations postgres "$$DATABASE_URL" down

sqlc:
	@echo "TODO: generate sqlc code"

lint:
	@echo "TODO: run linters"

check-ffmpeg:
	powershell -ExecutionPolicy Bypass -File ./scripts/check-ffmpeg.ps1

check-migrations:
	powershell -ExecutionPolicy Bypass -File ./scripts/check-migrations.ps1
