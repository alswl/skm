.PHONY: generate-code-wire
generate-code-wire: ## Generate wire injection code
	# wire
	@echo generate wire

	@for f in $(shell find . -name wire.go); do                                                        \
		(cd $$(dirname $$f); $$GOPATH/bin/wire);                                                          \
		echo "generate wire for $$(dirname $$f)";                                                         \
	done
