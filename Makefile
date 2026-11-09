test:
	docker run --rm -v $(CURDIR):/w openpolicyagent/opa:1.22.0 test /w/policy
