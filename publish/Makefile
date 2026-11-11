GO_IMAGE := golang:1.26-alpine
OWNER := $(shell id -u):$(shell id -g)
GATEWAY_PROTO := https://raw.githubusercontent.com/PTW-TUD/Ecosystems-Gateway/nautilus_v2/src/_proto/spp_v3.proto
# the repo root is mounted, as the tests read ../samples
DOCKER_GO := docker run --rm -v $(CURDIR)/..:/w -w /w/publish -e GOFLAGS=-buildvcs=false \
	-v dpp-demo-gomod:/go/pkg/mod -v dpp-demo-gocache:/root/.cache/go-build $(GO_IMAGE)

.PHONY: proto proto-sync tidy test image

# Generates gen/ from proto/ (needs only docker)
proto:
	$(DOCKER_GO) sh -c 'apk add --no-cache protobuf-dev >/dev/null && \
	  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11 && \
	  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.0 && \
	  rm -rf gen && mkdir gen && \
	  protoc -I proto -I /usr/include \
	    --go_out=gen --go_opt=module=dpp-demo/publish/gen \
	    --go-grpc_out=gen --go-grpc_opt=module=dpp-demo/publish/gen \
	    $$(find proto -name "*.proto") && chown -R $(OWNER) gen'

# Refreshes the vendored gateway proto from the nautilus_v2 branch, then regenerates
proto-sync:
	curl -fsSL $(GATEWAY_PROTO) | python3 -I scripts/strip_proto_options.py > proto/ecosystemsgateway/v3/spp_v3.proto
	$(MAKE) proto

tidy:
	$(DOCKER_GO) sh -c 'go mod tidy && chown $(OWNER) go.mod go.sum'

test:
	$(DOCKER_GO) sh -c 'go vet ./... && go test ./...'

image:
	docker build -t dpp-demo-publish .
