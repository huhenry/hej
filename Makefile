GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
BINARY_NAME=hej
BIN_PATH=bin
KUBECTL := kubectl

DOCKER := docker
DOCKER_SUPPORTED_VERSIONS ?= 17|18|19

REGISTRY_PREFIX ?= henryhucn

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# set the version number. you should not need to do this
# for the majority of scenarios.
ifeq ($(origin VERSION), undefined)
VERSION := $(shell git describe --dirty="-dev" --always --tags | sed 's/-/./2' | sed 's/-/./2' )
endif
export VERSION


# docker tag SOURCE_IMAGE[:TAG] swr.troila.com/library/IMAGE[:TAG]
# Image URL to use all building/pushing image targets
IMG ?= $(REGISTRY_PREFIX)/hej:$(VERSION)

GO := go
GIT_SHA=$(shell git rev-parse HEAD)
DATE=$(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
BUILD_INFO_IMPORT_PATH=github.com/huhenry/hej/pkg/version
BUILD_FLAGS="-X $(BUILD_INFO_IMPORT_PATH).commitSHA=$(GIT_SHA) -X $(BUILD_INFO_IMPORT_PATH).latestVersion=$(VERSION) -X $(BUILD_INFO_IMPORT_PATH).date=$(DATE)"
BUILD_INFO=-ldflags "-X $(BUILD_INFO_IMPORT_PATH).commitSHA=$(GIT_SHA) -X $(BUILD_INFO_IMPORT_PATH).latestVersion=$(VERSION) -X $(BUILD_INFO_IMPORT_PATH).date=$(DATE)"



GOFILES := $(shell find . -name "*.go" -type f -not -path "./vendor/*")
GOFMT ?= gofmt "-s"


.PHONY: build clean run all

all: build

hello:
		@echo "Hello"

build: go.build.linux_amd64.hej
	@echo "===========> Building binary successfully"

clean:
	    @echo cleaning 
	    rm -rf bin/$(BINARY_NAME) && $(GOCLEAN)

test:
	    $(GOTEST) ./...



.PHONY: fmt
fmt:
	@$(GOFMT) -w $(GOFILES)

.PHONY: fmt-check
fmt-check:
	@diff=$$($(GOFMT) -d $(GOFILES)); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make fmt' and commit the result:"; \
		echo "$${diff}"; \
		exit 1; \
	fi;

.PHONY: vet
vet:
	$(GO) vet $(PACKAGES)



.PHONY: go.compress.%
go.compress.%:
	$(eval COMMAND := $(word 2,$(subst ., ,$*)))
	$(eval PLATFORM := $(word 1,$(subst ., ,$*)))
	$(eval OS := $(word 1,$(subst _, ,$(PLATFORM))))
	$(eval ARCH := $(word 2,$(subst _, ,$(PLATFORM))))
	@echo "===========> Compressing binary $(COMMAND) $(VERSION) for $(OS) $(ARCH)"
	upx bin/$(COMMAND)

.PHONY: go.build.%
go.build.%:
	$(eval COMMAND := $(word 2,$(subst ., ,$*)))
	$(eval PLATFORM := $(word 1,$(subst ., ,$*)))
	$(eval OS := $(word 1,$(subst _, ,$(PLATFORM))))
	$(eval ARCH := $(word 2,$(subst _, ,$(PLATFORM))))
	@echo "===========> Building binary $(COMMAND) $(VERSION) for $(OS) $(ARCH) $(BUILD_INFO)"
	@mkdir -p bin/$(OS)/$(ARCH)
	@CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) $(GO) build -o bin/$(COMMAND) $(BUILD_INFO) cmd/$(COMMAND)/main.go 

.PHONY: image
image:	go.build.linux_amd64.hej	
	@echo "===========> Building hej $(VERSION) docker image"
	@$(DOCKER) build --pull -t $(REGISTRY_PREFIX)/hej:$(VERSION) .
	@echo "===========> Pushing hej $(VERSION) image to $(REGISTRY_PREFIX)"
	@$(DOCKER) push $(REGISTRY_PREFIX)/hej:$(VERSION)


.PHONY: dockerimage
dockerimage:
	@$(DOCKER) build --pull -t $(REGISTRY_PREFIX)/hej:$(VERSION) . -f Dockerfile.builder --build-arg GIT_SHA=$(GIT_SHA) --build-arg VERSION=$(VERSION) --build-arg DATE=$(DATE)

.PHONY: deploy
deploy:
	@echo "===========> update image $(IMG)"
	@$(KUBECTL) -n kube-system set image deployment hej hej=$(IMG)
	@$(KUBECTL) -n kube-system scale deployment hej --replicas=0
	@$(KUBECTL) -n kube-system scale deployment hej --replicas=1


deploy2dev:
	helm upgrade tpaas --version=2.3.3 --set hej.image.tag=$(VERSION) paas/tpaas --namespace=kube-system --reuse-values 
	@$(KUBECTL) -n kube-system scale deployment hej --replicas=0
	@$(KUBECTL) -n kube-system scale deployment hej --replicas=1