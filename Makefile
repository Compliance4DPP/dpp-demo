OPA = docker run --rm -v $(CURDIR):/w openpolicyagent/opa:1.22.0
# pinned so local runs and CI format identically; needs Node (npx)
PRETTIER = npx --yes prettier@3.3.3
.PHONY: test fmt fmt-check
test:
	$(OPA) test /w/policy
fmt:  # rewrite the policy files (opa fmt) and the web files (Prettier)
	$(OPA) fmt -w /w/policy
	$(PRETTIER) --write web
fmt-check:  # fail and list the files that are not formatted
	$(OPA) fmt --fail -l /w/policy
	$(PRETTIER) --check web
