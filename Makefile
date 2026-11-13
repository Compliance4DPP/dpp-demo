OPA = docker run --rm -v $(CURDIR):/w openpolicyagent/opa:1.22.0
# pinned so local runs and CI format identically; needs Node (npx)
PRETTIER = npx --yes prettier@3.3.3
# the publish service's Go toolchain; the repo root is mounted, as its tests read samples/
GO = docker run --rm -v $(CURDIR):/w -w /w/publish -e GOFLAGS=-buildvcs=false \
	-v dpp-demo-gocache:/root/.cache/go-build golang:1.26-alpine

.PHONY: test test_publish image_publish fmt fmt-check

# policy tests
test:
	$(OPA) test /w/policy

# publish service: go vet + go test
test_publish:
	$(GO) sh -c 'go vet ./... && go test ./...'

# publish service image on its own (docker compose --profile publish builds it as well)
image_publish:
	docker build -t dpp-demo-publish publish

# formatting: opa fmt for policy/, Prettier for web/, gofmt for publish/ (its templates are Go
# templates, which JSON and Markdown formatters would break, so they are left as written)
fmt:
	$(OPA) fmt -w /w/policy
	$(PRETTIER) --write web
	$(GO) gofmt -w .

fmt-check:
	$(OPA) fmt --fail -l /w/policy
	$(PRETTIER) --check web
	$(GO) sh -c 'files=$$(gofmt -l .); [ -z "$$files" ] || { echo "gofmt needed: $$files"; exit 1; }'
