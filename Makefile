PROJECT = "ecsmonitor"
BUILD_NUMBER = $(JENKINS_BUILD)

ifeq ($(BUILD_NUMBER), )
    BUILD_NUMBER = sudheer
endif

VERSION := 1.0.0
PACKER_VERSION := $(VERSION)-$(BUILD_NUMBER)
BUILDTAG := $(PACKER_VERSION).git$(shell git rev-parse --short=7 HEAD)
CONTAINER_ID := $(shell cat /proc/self/cgroup | grep 'docker' | sed 's/^.*\///' | tail -n1)

# --- Go ENV settings ---
export GO111MODULE=on
export GOEXPERIMENT=boringcrypto
export GOPRIVATE=github.com/cisco-sbg
export USERNM=jenkins
export GITCONFIG=~/.gitconfig:/home/$(USERNM)/.gitconfig:ro
export SSHCONFIG=~/.ssh:/home/$(USERNM)/.ssh:ro
export SCRIPTS=$(shell pwd)/scripts/build-fips-binary-local.sh:/home/$(USERNM)/build-fips-binary-local.sh
export ENTRYPOINT=/home/$(USERNM)/build-fips-binary-local.sh
export APP=$(shell pwd):/home/$(USERNM)/$(PROJECT)
export CONTAINERNAME=containers.cisco.com/ssedevops/base-ci-fips:latest_evtng

# --- Targets ---
install:
	@echo " > Building binary..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap

build:
	chmod 755 bootstrap
	zip deployment.zip bootstrap
	aws s3 cp --no-progress deployment.zip s3://sse-serverless-artifacts/ecsmonitor/artifacts/$(BUILDTAG)
	echo ecsTaskCheckerArtifactTag=$(BUILDTAG) > ecsmonitor.artifact
	echo ecsTaskCheckerArtifactType=serverless >> ecsmonitor.artifact

all: build test

test: fmtcheck
	@sh -c "'$$(pwd)/scripts/coverage.sh'"

testrace:
	go test -race $$(go list ./... | grep -v /vendor/)

vet:
	@go vet 2>/dev/null ; if [ $$? -eq 3 ]; then \
		go get code.google.com/p/go.tools/cmd/vet; \
	fi
	@echo "go vet $$(go list ./...) 2>&1 | tee vet.txt"
	@go vet $$(go list ./...) 2>&1 | tee vet.txt ; if [ $$? -eq 1 ]; then \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

fmtcheck:
	# @sh -c "'$$(pwd)/scripts/gofmtcheck.sh'"

fmt:
	gofmt -w $(shell find . -name '*.go' | grep -v vendor)

clean:
	go clean

deps: clean
	git config --global url."git@github.com:".insteadOf "https://github.com/"
	@go clean -modcache
	@go mod tidy

jenkins: vet test install build

build-fips-binary-local:
	docker run --rm \
		-v $(GITCONFIG) \
		-v $(SSHCONFIG) \
		-v $(APP) \
		-v $(SCRIPTS) \
		--entrypoint $(ENTRYPOINT) \
		-it $(CONTAINERNAME)
