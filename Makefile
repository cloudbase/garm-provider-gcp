SHELL := bash

ROOTDIR=$(dir $(abspath $(lastword $(MAKEFILE_LIST))))
GOPATH ?= $(shell go env GOPATH)
GO ?= go

IMAGE_TAG = garm-provider-build

USER_ID=$(shell ((docker --version | grep -q podman) && echo "0" || id -u))
USER_GROUP=$(shell ((docker --version | grep -q podman) && echo "0" || id -g))
GARM_PROVIDER_NAME := garm-provider-gcp

default: build

.PHONY : build build-static test lint go-test fmt fmtcheck verify-vendor verify create-release-files release

build:
	@$(GO) build .

clean: ## Clean up build artifacts
	@rm -rf ./bin ./build ./release

build-static:
	@echo Building
	docker build --tag $(IMAGE_TAG) .
	mkdir -p build
	docker run --rm -e GARM_PROVIDER_NAME=$(GARM_PROVIDER_NAME) -e USER_ID=$(USER_ID) -e USER_GROUP=$(USER_GROUP) -v $(PWD)/build:/build/output:z -v $(PWD):/build/$(GARM_PROVIDER_NAME):z $(IMAGE_TAG) /build-static.sh
	@echo Binaries are available in $(PWD)/build

test: golangci-lint verify go-test

lint: golangci-lint
	@$(GOLANGCI_LINT) run --timeout=8m --build-tags testing

go-test:
	@$(GO) test -race -mod=vendor -tags testing -v $(TEST_ARGS) -timeout=15m -parallel=4 -count=1 ./...

fmt:
	@$(GO) fmt $$(go list ./...)

fmtcheck:
	@gofmt -l -s $$(go list ./... | sed -n 's/github.com\/cloudbase\/'$(GARM_PROVIDER_NAME)'\/\(.*\)/\1/p') | grep ".*\.go"; if [ "$$?" -eq 0 ]; then echo "gofmt check failed; please tun gofmt -w -s"; exit 1;fi

verify-vendor: ## verify if all the go.mod/go.sum files are up-to-date
	$(eval TMPDIR := $(shell mktemp -d))
	@cp -R ${ROOTDIR} ${TMPDIR}
	@(cd ${TMPDIR}/$(GARM_PROVIDER_NAME) && ${GO} mod tidy)
	@diff -r -u -q ${ROOTDIR} ${TMPDIR}/$(GARM_PROVIDER_NAME) >/dev/null 2>&1; if [ "$$?" -ne 0 ];then echo "please run: go mod tidy && go mod vendor"; exit 1; fi
	@rm -rf ${TMPDIR}

verify: verify-vendor lint fmtcheck

##@ Release
create-release-files:
	./scripts/make-release.sh

release: build-static create-release-files ## Create a release

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
GOLANGCI_LINT ?= $(LOCALBIN)/golangci-lint

## Tool Versions
GOLANGCI_LINT_VERSION ?= v1.64.8

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary. If wrong version is installed, it will be overwritten.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint && $(LOCALBIN)/golangci-lint --version | grep -q $(GOLANGCI_LINT_VERSION) || \
        GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

