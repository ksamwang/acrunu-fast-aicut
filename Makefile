.PHONY: dev infra test migrate-up migrate-down sqlc lint check-ffmpeg

dev:
	docker compose up --build

infra:
	docker compose up postgres redis

test:
	@echo "TODO: run tests"

migrate-up:
	@echo "TODO: run database migrations"

migrate-down:
	@echo "TODO: rollback database migrations"

sqlc:
	@echo "TODO: generate sqlc code"

lint:
	@echo "TODO: run linters"

check-ffmpeg:
	powershell -ExecutionPolicy Bypass -File ./scripts/check-ffmpeg.ps1
