.PHONY: run tidy up down logs

run:
	@godotenv go run ./cmd/server

tidy:
	go mod tidy

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f
