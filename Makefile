TARGETS := $(shell ls scripts)

help:
	@echo "Available make targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Auto-generated targets from scripts/:"
	@for target in $(TARGETS); do \
		if echo "$$target" | grep -qE '\.txt$$'; then continue; fi; \
		desc=$$(grep -m1 '^# DESC:' scripts/$$target | sed 's/^# DESC:[[:space:]]*//'); \
		if [ -z "$$desc" ]; then \
			desc="(no description)"; \
		fi; \
		printf "  \033[36m%-20s\033[0m %s\n" "$$target" "$$desc"; \
	done


.dapper:
	@echo Downloading dapper
	@curl -sL https://releases.rancher.com/dapper/latest/dapper-$$(uname -s)-$$(uname -m) > .dapper.tmp
	@@chmod +x .dapper.tmp
	@./.dapper.tmp -v
	@mv .dapper.tmp .dapper

$(TARGETS): .dapper
	./.dapper $@

.DEFAULT_GOAL := default

.PHONY: $(TARGETS)
