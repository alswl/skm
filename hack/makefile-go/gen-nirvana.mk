NIRVANA_BIN = $$HOME/local/bin/nirvana
.PHONY: generate-code-openapiv3
generate-code-openapiv3:  ## Generate OpenAPIV3
	@for target in $(TARGETS); do                                                                                        \
		if [ -f configs/$${target}/nirvana-it.yaml ]; then                                                               \
		  echo "# overwrite nirvana.yaml";                                                                               \
		  cp configs/$${target}/nirvana-it.yaml nirvana.yaml;                                                            \
        fi;                                                                                                              \
		if [ -d pkg/web/$${target} ]; then                                                                               \
			echo "# generate pkg/web/$${target}";                                                                        \
			$(NIRVANA_BIN) api pkg/web/$${target} --serve= --output=/tmp --escape-class-name-symbol --open-api-v3;       \
			mv /tmp/api.v1.json api/openapi/$${target}-api.json;                                                         \
		fi;                                                                                                              \
	done
