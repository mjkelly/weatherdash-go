.PHONY: run
run: ## Run the server on port 8080
	go build && ./weatherdash-go

.PHONY: clean
clean: ## Clean up generated files
	rm -rf weatherdash-go

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
