.PHONY: container-build-env
newBuildEnvVersion=$(GO_MOD_VERSION)-$(shell tar cf - build/$(PROJECT) | sha256sum | cut -c1-6)
container-build-env: check-git-status ## Build container build env
	@for registry in $(REGISTRIES); do                                                                              \
	  image=$(IMAGE_PREFIX)$(PROJECT)$(IMAGE_SUFFIX);                                                               \
	    docker-buildx build --provenance=false -t $${registry}$${image}-build-env:$(newBuildEnvVersion)             \
	      --platform linux/amd64                                                                                    \
	      --build-arg ROOT=$(ROOT)                                                                                      \
	      --build-arg CMD_DIR=$(CMD_DIR)                                                                            \
	      --build-arg VERSION=$(GO_MOD_VERSION)                                                                     \
	      --build-arg COMMIT=$(COMMIT)                                                                              \
	      --build-arg CGO_ENABLED=$(CGO_ENABLED)                                                                    \
	    --progress=plain                                                                                            \
	    -f $(BUILD_DIR)/$(PROJECT)/Dockerfile .;                                                                    \
	done


.PHONY: bump-build-env-container
newBuildEnvVersion=$(GO_MOD_VERSION)-$(shell tar cf - build/$(PROJECT) | sha256sum | cut -c1-6)
bump-build-env-container: ## Bump build env container for all containers and aci
	echo "$(newBuildEnvVersion)" > build/$(PROJECT)/VERSION
	gsed -E -i "s/build-env:[0-9A-Za-z]{6}-[0-9A-Za-z]{6}/build-env:$(newBuildEnvVersion)/g" .aci.yml
	find build -name "Dockerfile" -exec gsed -E -i "s/build-env:[0-9A-Za-z]{6}-[0-9A-Za-z]{6}/build-env:$(newBuildEnvVersion)/g" {} \;
	@echo "# PLEASE using 'git commit -a' to commit image version changes"


.PHONY: push-container-build-env
newBuildEnvVersion=$(GO_MOD_VERSION)-$(shell tar cf - build/$(PROJECT) | sha256sum | cut -c1-6)
push-container-build-env: ## Push containers build env images to registry
	@for registry in $(REGISTRIES); do                                                                 \
	  image=$(IMAGE_PREFIX)$(PROJECT)$(IMAGE_SUFFIX);                                                  \
	    docker push $${registry}$${image}-build-env:$(newBuildEnvVersion);                             \
	done

