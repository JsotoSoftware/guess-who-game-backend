.PHONY: run tidy up down logs migrate-up migrate-down migrate-force

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

get-dsn:
	@powershell -Command "if (-not (Test-Path .env)) { Write-Host 'Error: .env file not found'; exit 1 }; (Get-Content .env | Select-String '^POSTGRES_DSN=').Line -replace '^POSTGRES_DSN=', ''"

migrate-up:
	@powershell -Command "$$dsn = (Get-Content .env | Select-String '^POSTGRES_DSN=').Line -replace '^POSTGRES_DSN=', ''; if ([string]::IsNullOrEmpty($$dsn)) { Write-Host 'Error: POSTGRES_DSN not found in .env file'; exit 1 }; $$dsn = $$dsn -replace 'localhost', 'postgres'; docker compose run --rm migrate -path /migrations -database \"$$dsn\" up"

migrate-down:
	@powershell -Command "$$dsn = (Get-Content .env | Select-String '^POSTGRES_DSN=').Line -replace '^POSTGRES_DSN=', ''; if ([string]::IsNullOrEmpty($$dsn)) { Write-Host 'Error: POSTGRES_DSN not found in .env file'; exit 1 }; $$dsn = $$dsn -replace 'localhost', 'postgres'; docker compose run --rm migrate -path /migrations -database \"$$dsn\" down 1"

migrate-force:
	@powershell -Command "if ([string]::IsNullOrEmpty('$(V)')) { Write-Host 'Error: V (version) is required. Usage: make migrate-force V=1'; exit 1 }; $$dsn = (Get-Content .env | Select-String '^POSTGRES_DSN=').Line -replace '^POSTGRES_DSN=', ''; if ([string]::IsNullOrEmpty($$dsn)) { Write-Host 'Error: POSTGRES_DSN not found in .env file'; exit 1 }; $$dsn = $$dsn -replace 'localhost', 'postgres'; docker compose run --rm migrate -path /migrations -database \"$$dsn\" force $(V)"