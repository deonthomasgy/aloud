.PHONY: run build test app docker-up docker-down kokoro-up kokoro-down kokoro-status

run:
	go run .

build:
	go build -o invtts .

app:
	chmod +x macos/build-app.sh && ./macos/build-app.sh

test:
	go test ./...

docker-up:
	docker compose up --build -d

docker-down:
	docker compose down

kokoro-up:
	./scripts/kokoro-alpha-old.sh up

kokoro-down:
	./scripts/kokoro-alpha-old.sh down

kokoro-status:
	./scripts/kokoro-alpha-old.sh status
