.PHONY: generate-code
generate-code: ## deprecated , using others generate-code-x
	@echo -n ''


.PHONY: generate-code-swagger
generate-code-swagger: ## generate code from swagger
	@echo generate swagger
	# TODO enable validation
	@(cd pkg; rm client/zz_generated_*.go; rm client/*/zz_generated_*.go; rm models/zz_generated_*.go; swagger generate client -f ../api/openapi.yaml -A $(PROJECT) -C ../api/config.yaml --skip-validation)
	# TODO waiting upstream merge


.PHONY: generate-code-mockery
generate-code-mockery: ## Run generate generated unit test code
	# 如果遇到问题
	# Unexpected package creation during export data loading
	# https://github.com/vektra/mockery/pull/435#issuecomment-1134329306
	@echo "# generate mock of interfaces for testing"
	@rm -rf test/mock
	@mkdir -p test/mock
	@(cd . && mockery --all --keeptree --case=underscore --packageprefix=mock --output=./test/mock/)
	# mockery not support 1.18 generic now, temporarily drop zero size golang file
	# https://github.com/vektra/mockery/pull/456
	find test/mock -size 0 -exec rm {} \;


generate-code-enum: ## Generate enum String for models
	@echo generate stringer for enums
	# TODO using ls
	# @(cd pkg/models/enums/component/; go generate)


CMD_DOCS_DIR ?= docs

.PHONY: generate-manual
generate-manual: ## Generate develop docs
	@# go install github.com/princjef/gomarkdoc/cmd/gomarkdoc@latest
	@# gomarkdoc --output MANUAL.md your-host.com/your/your-project

	@# if contains go files in .
	@if [ -n "$(shell ls *.go 2>/dev/null)" ]; then                                                    \
		echo "generate manual";                                                                           \
		gomarkdoc --output MANUAL.md .;                                                                   \
	fi
	@if [ -d "cmd/gendoc/" ]; then                                                                     \
		echo "generate cmd doc";                                                                          \
		mkdir -p $(CMD_DOCS_DIR);                                                                         \
		CMD_DOCS_DIR=$(CMD_DOCS_DIR) go run ./cmd/gendoc;                                                 \
	fi

.PHONY: generate-gorm
generate-gorm:
	@if [ -d "cmd/dalgen/" ]; then                                                                     \
		echo "generate gorm";                                                                             \
		go run $(ROOT)/cmd/dalgen;                                                                        \
	fi
