.DEFAULT_GOAL := help

.PHONY: help build install build-windows test clean

help: ## Show available targets
	@echo "pw – project switcher"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Build pw binary for current OS/arch
	go build -o pw .

install: build ## Build and install to ~/go/bin
	mkdir -p ~/go/bin
	cp pw ~/go/bin/pw

build-windows: ## Cross-compile pw.exe for Windows (amd64)
	GOOS=windows GOARCH=amd64 go build -o pw.exe .

test: ## Run all tests
	go test ./...

clean: ## Remove built binaries
	rm -f pw pw.exe
