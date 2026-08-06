.PHONY: check-git-status
check-git-status: ## Check git status
	@test -z "$$(git status --porcelain)" || (echo "Git status is not clean, please commit or stash your changes first" && exit 1)
