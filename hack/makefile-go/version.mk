DEFAULT_BUMP_STAGE=beta # final|alpha|beta|candidate
DEFAULT_BUMP_SCOPE=minor # major|minor|patch
DEFAULT_BUMP_DRY_RUN=true # true|false

STAGE=$(DEFAULT_BUMP_STAGE)
SCOPE=$(DEFAULT_BUMP_SCOPE)
DRY_RUN=$(DEFAULT_BUMP_DRY_RUN)
PUSH ?= false

# Post-actions script run after VERSION is written (e.g. sync to pyproject.toml / package.json).
# Auto-detected from hack/bump-post-actions.sh; override POST_ACTIONS_SCRIPT to customize.
POST_ACTIONS_SCRIPT ?= ./hack/bump-post-actions.sh
BUMP_POST_ACTIONS := $(if $(wildcard $(POST_ACTIONS_SCRIPT)),--post-actions-script $(POST_ACTIONS_SCRIPT),)

.PHONY: bump-patch
bump-patch: check-git-status ## Bump patch version (final)
	(bash ./hack/bump.sh --stage final --scope patch --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

.PHONY: bump-minor
bump-minor: check-git-status ## Bump minor version (final)
	(bash ./hack/bump.sh --stage final --scope minor --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

.PHONY: bump-major
bump-major: check-git-status ## Bump major version (final)
	(bash ./hack/bump.sh --stage final --scope major --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

.PHONY: bump-beta
bump-beta: check-git-status ## Bump beta pre-release (minor)
	(bash ./hack/bump.sh --stage beta --scope ${SCOPE} --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

.PHONY: bump-alpha
bump-alpha: check-git-status ## Bump alpha pre-release (minor)
	(bash ./hack/bump.sh --stage alpha --scope ${SCOPE} --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

.PHONY: bump
bump: check-git-status ## Bump version (uses STAGE and SCOPE variables)
	(bash ./hack/bump.sh --stage ${STAGE} --scope ${SCOPE} --dry-run ${DRY_RUN} --push ${PUSH} ${BUMP_POST_ACTIONS})

SUB ?=
.PHONY: bump-sub
bump-sub: check-git-status ## Bump sub-module version
	@test -n "$(SUB)" || (echo "SUB is required. Usage: make bump-sub SUB=<dir> [SCOPE=patch]" && exit 1)
	(bash ./hack/bump-sub-mod.sh $(SUB) ${SCOPE} ${DRY_RUN} ${PUSH})

.PHONY: version
version: ## Show current version
	@cat VERSION

.PHONY: gen-changelog-patch
gen-changelog-patch: ## Generate changelog for next patch version
	(bash ./hack/gen-changelog.sh --stage final --scope patch)

.PHONY: gen-changelog-minor
gen-changelog-minor: ## Generate changelog for next minor version
	(bash ./hack/gen-changelog.sh --stage final --scope minor)

.PHONY: gen-changelog-major
gen-changelog-major: ## Generate changelog for next major version
	(bash ./hack/gen-changelog.sh --stage final --scope major)

.PHONY: gen-changelog-beta
gen-changelog-beta: ## Generate changelog for next beta version
	(bash ./hack/gen-changelog.sh --stage beta --scope ${SCOPE})

.PHONY: gen-changelog-alpha
gen-changelog-alpha: ## Generate changelog for next alpha version
	(bash ./hack/gen-changelog.sh --stage alpha --scope ${SCOPE})

.PHONY: gen-changelog
gen-changelog: ## Generate changelog (uses STAGE and SCOPE variables)
	(bash ./hack/gen-changelog.sh --stage ${STAGE} --scope ${SCOPE})
