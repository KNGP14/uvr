GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

VERSION=$(shell git describe --exact-match --tags 2>/dev/null)

BUILD_DIR=build

PACKAGE_UVR2JSON_ARM=uvr2json-$(VERSION)_linux_arm
PACKAGE_UVR2JSON_AMD64=uvr2json-$(VERSION)_linux_amd64

unexport GOPATH

all: test build
build:
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -i $(BUILD_SRC)

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)

package-uvr2json-arm: build-uvr2json-arm
	tar -cvzf $(PACKAGE_UVR2JSON_ARM).tar.gz -C $(BUILD_DIR) $(PACKAGE_UVR2JSON_ARM)

build-uvr2json-arm:
	GOOS=linux GOARCH=arm GOARM=6 $(GOBUILD) -o $(BUILD_DIR)/$(PACKAGE_UVR2JSON_ARM)/uvr2json cmd/uvr2json/main.go

build-uvr2json-amd64:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BUILD_DIR)/$(PACKAGE_UVR2JSON_AMD64)/uvr2json cmd/uvr2json/main.go
