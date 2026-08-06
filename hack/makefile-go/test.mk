.PHONY: test
test: ## Run unit tests with coverage
	@# NOTICE, the test output is using for coverage analytics, did not modify the std out
	@echo "cover package: ${UT_COVER_PACKAGES}"
	# gcflags disable inlining and loop unrolling, for mock lib
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go test -gcflags="all=-l -N" -v ${UT_COVER_PACKAGES} -race -coverprofile cover.out -tags="!integration,!e2e" ./...
	@go tool cover -func cover.out | tail -n 1 # print UT total coverage

.PHONY: test-raw
test-raw: ## Run unit tests without coverage(but all ut)
	# gcflags disable inlining and loop unrolling, for mock lib
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go test -gcflags="all=-l -N" -v -race -tags="!integration,!e2e" ./...

.PHONY: integration-test
integration-test: ## Run integration tests
	@echo "cover package: ${IT_COVER_PACKAGES}"
	@if [ -d "./test/suites" ]; then                                                                   \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go test -gcflags="all=-l -N" -v ${IT_COVER_PACKAGES} -coverprofile cover.out -race -tags=integration ./test/suites/...;  \
		go tool cover -func cover.out | tail -n 1;                                                        \
	fi

.PHONY: e2e-test
e2e-test: ## Run e2e tests
	@if [ -d "./test/e2e" ]; then                                                                      \
		GOOS=$(GOOS) GOARCH=$(GOARCH) go test -gcflags="all=-l -N" -coverprofile cover.out -tags=e2e ./test/e2e/...;  \
		go tool cover -func cover.out | tail -n 1;                                                        \
	fi

# Reference https://go.dev/testing/coverage/
.PHONY: show-coverage
show-coverage: ##  show coverage of UT and IT in specific packages
# No skip, run all tests
ifneq ($(skip), true)
	@# go version should be greater than @1.20
	@mkdir -p ${COVERAGE_PROFILING_DIR}
	@rm -rf ${COVERAGE_PROFILING_DIR}/*
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go test -cover -coverpkg=$(COVERAGE_PACKAGES) -tags="!integration,!e2e" ${UT_COVER_PACKAGES} -args -test.gocoverdir=${COVERAGE_PROFILING_DIR} # Unit test
	@GOOS=$(GOOS) GOARCH=$(GOARCH) go test -cover -coverpkg=$(COVERAGE_PACKAGES) -tags=integration ${IT_COVER_PACKAGES} -args -test.gocoverdir=${COVERAGE_PROFILING_DIR}	# Integration test
endif
	@go tool covdata func -i $(COVERAGE_PROFILING_DIR) -pkg $(COVERAGE_PACKAGES)| tail -n 1
