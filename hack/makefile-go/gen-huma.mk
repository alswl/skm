.PHONY: generate-code-openapiv3-huma
generate-code-openapiv3-huma: build ## Generate OpenAPI v3 from Huma API
	@# Huma serves OpenAPI at /openapi by default.
	@for target in $(TARGETS); do                                                                      \
		echo "# generate OpenAPI for $${target}";                                                         \
		$(OUTPUT_DIR)/$${target} &                                                                        \
		_PID=$$!;                                                                                         \
		for i in 1 2 3 4 5; do                                                                            \
			curl -sS -o api/openapi/$${target}-api.json http://localhost:8888/openapi 2>/dev/null            \
				&& break;                                                                                       \
			sleep 1;                                                                                         \
		done;                                                                                             \
		kill $$_PID 2>/dev/null;                                                                          \
		wait $$_PID 2>/dev/null;                                                                          \
	done
