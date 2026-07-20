TAG_NAME := $(shell git describe --abbrev=0 --tags --exact-match 2>/dev/null || true)
SHA := $(shell git rev-parse HEAD)
VERSION_GIT := $(if $(TAG_NAME),$(TAG_NAME),$(SHA))
VERSION := $(if $(VERSION),$(VERSION),$(VERSION_GIT))

BIN_NAME := traefik
CODENAME ?= cheddar
DATE := $(shell date -u '+%Y-%m-%d_%I:%M:%S%p')

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOGC ?=

IMAGE_NAME ?= traefik/traefik:latest

.PHONY: binary-with-webui
#? binary-with-webui: Rebuild the Vue WebUI assets and compile a Traefik binary with embedded frontend resources
binary-with-webui:
	rm -rf webui/static
	mkdir -p webui/static
	printf 'For more information see `webui/readme.md`' > webui/static/DONT-EDIT-FILES-IN-THIS-DIRECTORY.md
	docker build -t traefik-webui -f webui/buildx.Dockerfile webui
	docker run --rm -v "$(PWD)/webui/static":'/src/webui/static' traefik-webui npm run build:prod
	docker run --rm -v "$(PWD)/webui/static":'/src/webui/static' traefik-webui chown -R $(shell id -u):$(shell id -g) ./static
	printf 'For more information see `webui/readme.md`' > webui/static/DONT-EDIT-FILES-IN-THIS-DIRECTORY.md
	mkdir -p dist
	@echo SHA: $(VERSION) $(CODENAME) $(DATE)
	CGO_ENABLED=0 GOGC=${GOGC} GOOS=${GOOS} GOARCH=${GOARCH} go build ${FLAGS} -ldflags "-s -w \
    -X github.com/traefik/traefik/v3/pkg/version.Version=$(VERSION) \
    -X github.com/traefik/traefik/v3/pkg/version.Codename=$(CODENAME) \
    -X github.com/traefik/traefik/v3/pkg/version.BuildDate=$(DATE)" \
    -installsuffix nocgo -o "./dist/${GOOS}/${GOARCH}/$(BIN_NAME)" ./cmd/traefik

.PHONY: image-with-webui
#? image-with-webui: Rebuild the Vue WebUI assets, compile the Traefik binary, and build a Docker image containing the embedded frontend resources
image-with-webui:
	@$(MAKE) binary-with-webui
	docker buildx build --load -t $(IMAGE_NAME) --platform=linux/$(GOARCH) -f Dockerfile .
