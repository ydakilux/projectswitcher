.DEFAULT_GOAL := help

.PHONY: help build install build-windows test clean

help: ## Show available targets
	@echo "pw – project switcher"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Build pw binary for current OS/arch
	go build -o pw .

install: build ## Build and install to ~/go/bin and hook into shell rc files
	mkdir -p ~/go/bin
	cp pw ~/go/bin/pw
	cp shell/pw.bash shell/pw.zsh shell/pw.fish ~/go/bin/
	@if [ -f "$$HOME/.bashrc" ]; then \
		line='source ~/go/bin/pw.bash'; \
		if ! grep -qF "$$line" "$$HOME/.bashrc"; then \
			printf '\n# pw project switcher\n%s\n' "$$line" >> "$$HOME/.bashrc"; \
			echo "Added pw hook to ~/.bashrc"; \
		else \
			echo "pw hook already in ~/.bashrc"; \
		fi; \
	fi
	@if [ -f "$$HOME/.zshrc" ]; then \
		line='source ~/go/bin/pw.zsh'; \
		if ! grep -qF "$$line" "$$HOME/.zshrc"; then \
			printf '\n# pw project switcher\n%s\n' "$$line" >> "$$HOME/.zshrc"; \
			echo "Added pw hook to ~/.zshrc"; \
		else \
			echo "pw hook already in ~/.zshrc"; \
		fi; \
	fi
	@fish_conf="$$HOME/.config/fish/config.fish"; \
	if [ -f "$$fish_conf" ]; then \
		line='source ~/go/bin/pw.fish'; \
		if ! grep -qF "$$line" "$$fish_conf"; then \
			printf '\n# pw project switcher\n%s\n' "$$line" >> "$$fish_conf"; \
			echo "Added pw hook to $$fish_conf"; \
		else \
			echo "pw hook already in $$fish_conf"; \
		fi; \
	fi

build-windows: ## Cross-compile pw.exe for Windows (amd64)
	GOOS=windows GOARCH=amd64 go build -o pw.exe .

test: ## Run all tests
	go test ./...

clean: ## Remove built binaries
	rm -f pw pw.exe
