.PHONY: unit test

unit:
	ginkgo $(COMMAND) -r --skipPackage=testing/integration $(PACKAGE)

test:
	ginkgo $(COMMAND) -r
