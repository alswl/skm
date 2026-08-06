.PHONY: container
container: ## Build containers
	# COMMIT pass to container, because no git repo in container
	@for target in $(TARGETS); do                                                                      \
	  for registry in $(REGISTRIES); do                                                                \
	    image=$(IMAGE_PREFIX)$${target}$(IMAGE_SUFFIX);                                                \
	    docker-buildx build --provenance=false -t $${registry}$${image}:$(VERSION)                     \
	      --platform linux/amd64                                                                       \
	      --build-arg COMMIT=$(COMMIT)                                                                 \
	      --build-arg CGO_ENABLED=$(CGO_ENABLED)                                                       \
	      --progress=plain                                                                             \
	      -f $(BUILD_DIR)/$${target}/Dockerfile .;                                                     \
	  done                                                                                             \
	done


.PHONY: push-container
push-container: ## Push containers images to registry
	@for target in $(TARGETS); do                                                                      \
	  for registry in $(REGISTRIES); do                                                                \
	    image=$(IMAGE_PREFIX)$${target}$(IMAGE_SUFFIX);                                                \
	    docker push $${registry}$${image}:$(VERSION);                                                  \
	  done                                                                                             \
	done
