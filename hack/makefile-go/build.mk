.PHONY: download
download: ## Run go mod download
	go mod download


.PHONY: fmt
fmt: ## Run format code
	gofmt -w .
	go fmt ./...
	# vet is run by golangci-lint
	# https://golangci-lint.run/usage/linters/
	# go vet ./...
	goimports -w .
	golangci-lint run --fix


.PHONY: lint
lint: ## Run lint
	@echo "# gofmt"
	@test $$(gofmt -l . | wc -l) -eq 0

	@echo "# ensure integration test with // +build integration"
	@if [ -n "$$(find . -name '*_integration_test.go')" ]; then                                                                                                  \
		test $$(find . -name '*_integration_test.go' | wc -l) -eq $$(cat $$(find . -name '*_integration_test.go') | grep -E '// ?\+build integration' | wc -l);  \
	fi

	go mod tidy
	gofmt -w .
	golangci-lint run --timeout 5m

	# (cd examples && go mod tidy && golangci-lint run --timeout 5m && gofmt -w .)


.PHONY: clean
clean: ## Clean temp files
	@rm -vrf ${OUTPUT_DIR}/*


.PHONY: build
build: ## Build
	@for target in $(TARGETS); do                                                                               \
	  GOOS=$(GOOS) GOARCH=$(GOARCH) go build -v -o $(OUTPUT_DIR)/$${target}-$(GOOS)-$(GOARCH)                   \
	    $(if $(GO_BUILD_TAGS),-tags=$(GO_BUILD_TAGS),)                                                          \
	    -ldflags "-s -w                                                                                         \
	    -X $(ROOT)/pkg/version.Version=$(VERSION)                                                               \
	    -X $(ROOT)/pkg/version.Commit=$(COMMIT)                                                                 \
	    -X $(ROOT)/pkg/version.Package=$(ROOT)                                                                  \
	    -X $(ROOT)/internal/version.Version=$(VERSION)                                                          \
	    -X $(ROOT)/internal/version.Commit=$(COMMIT)                                                            \
	    -X $(ROOT)/internal/version.Package=$(ROOT)"                                                            \
	    $(CMD_DIR)/$${target};                                                                                  \
	  cp $(OUTPUT_DIR)/$${target}-$(GOOS)-$(GOARCH) $(OUTPUT_DIR)/$${target};                                   \
	  cp $(OUTPUT_DIR)/$${target}-$(GOOS)-$(GOARCH) $(OUTPUT_DIR)/$${target}-$(GOOS)-$(GOARCH)-$(VERSION);      \
	  cp $(OUTPUT_DIR)/$${target}-$(GOOS)-$(GOARCH) $(OUTPUT_DIR)/$${target}-$(VERSION);                        \
	done


.PHONY: compress
compress: ## Compress by upx
	@# move out of loop to avoid syntax error
	@# upx not works in debian for security reason
	@# upx $(OUTPUT_DIR)/$${target};
	@for target in $(TARGETS); do                                                                      \
	    echo "";                                                                                       \
	done
