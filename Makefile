.PHONY: all web server cli build dev run test vet clean

all: build

## Build the frontend into apps/web/dist (embedded by the server build).
web:
	cd apps/web && npm install && npm run build

## Build the server binary (embeds apps/web/dist).
server:
	go build -o bin/obsidianweb ./apps/server

## Build the CLI binary.
cli:
	go build -o bin/obsidianweb-cli ./apps/cli

## Full production build: frontend + single server binary + CLI.
build: web server cli

## Run the backend against the demo vault (API on :8787).
run:
	go run ./apps/server -config config.yaml

## Frontend dev server with API proxy (run `make run` in another terminal).
dev:
	cd apps/web && npm run dev

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin apps/web/dist/*
	touch apps/web/dist/.gitkeep
