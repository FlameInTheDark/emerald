.DEFAULT_GOAL := build

APP_NAME := emerald$(if $(filter Windows_NT,$(OS)),.exe,)
WEB_DIR := web
WEB_DEPS := $(WEB_DIR)/node_modules/.install-stamp
EMBED_DIR := internal/api/web/dist

export CGO_ENABLED := 1

ifeq ($(OS),Windows_NT)
RM_BINARY = if exist "$(APP_NAME)" del /q "$(APP_NAME)"
RM_EMBED = if exist "$(EMBED_DIR)" rmdir /s /q "$(EMBED_DIR)"
WRITE_WEB_DEPS = type nul > node_modules\.install-stamp
else
RM_BINARY = rm -f "$(APP_NAME)"
RM_EMBED = rm -rf "$(EMBED_DIR)"
WRITE_WEB_DEPS = touch node_modules/.install-stamp
endif

.PHONY: build build-web build-images run clean test lint docker docker-run

$(WEB_DEPS): $(WEB_DIR)/package.json $(WEB_DIR)/package-lock.json
	cd $(WEB_DIR) && npm ci && $(WRITE_WEB_DEPS)

build-images: $(WEB_DEPS)
	cd $(WEB_DIR) && npm run render:editor-illustrations

build-web: build-images
	cd $(WEB_DIR) && npm run build

build: build-web
	go build -ldflags="-s -w" -o $(APP_NAME) ./cmd/server

run: build-web
	go run ./cmd/server

clean:
	$(RM_BINARY)
	$(RM_EMBED)

test:
	go test -race -cover ./...

lint:
	golangci-lint run

docker:
	docker build -t emerald .

docker-run:
	docker-compose up -d
