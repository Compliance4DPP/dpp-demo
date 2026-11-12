OPA = docker run --rm -v $(CURDIR):/w openpolicyagent/opa:1.22.0
# pinned so local runs and CI format identically; needs Node (npx)
PRETTIER = npx --yes prettier@3.3.3
.PHONY: test test-js fmt fmt-check
test: test-js
	$(OPA) test /w/policy
test-js:
	node --test test/
fmt:
	$(OPA) fmt -w /w/policy
	$(PRETTIER) --write web test
fmt-check:
	$(OPA) fmt --fail -l /w/policy
	$(PRETTIER) --check web test
