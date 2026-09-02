.DEFAULT_GOAL := help

.PHONY: help build install build-windows test clean

# Detect Windows so targets work whether make is invoked from Git Bash/WSL
# (where SHELL is sh/bash) or from a native Windows make (cmd.exe as SHELL).
ifeq ($(OS),Windows_NT)
	EXE := .exe
	# Force recipes through cmd.exe. Without this, some native Windows make
	# builds (e.g. GnuWin32/mingw32-make) run simple recipe lines via a raw
	# CreateProcess instead of a shell, which skips PATH/PATHEXT resolution
	# and fails to find tools like `go` even when they're on PATH.
	SHELL := cmd.exe
	.SHELLFLAGS := /c
else
	EXE :=
endif

help: ## Show available targets
ifeq ($(OS),Windows_NT)
	@echo pw - project switcher
	@echo   help            Show available targets
	@echo   build           Build pw binary for current OS/arch
	@echo   install         Build and install (use install.ps1 on Windows)
	@echo   build-windows   Cross-compile pw.exe for Windows (amd64)
	@echo   test            Run all tests
	@echo   clean           Remove built binaries
else
	@echo "pw - project switcher"
	@echo "  help            Show available targets"
	@echo "  build           Build pw binary for current OS/arch"
	@echo "  install         Build and install (POSIX shells; use install.ps1 on Windows)"
	@echo "  build-windows   Cross-compile pw.exe for Windows (amd64)"
	@echo "  test            Run all tests"
	@echo "  clean           Remove built binaries"
endif

build: ## Build pw binary for current OS/arch
ifeq ($(OS),Windows_NT)
	cmd /c go build -o pw$(EXE) .
else
	go build -o pw$(EXE) .
endif

install: build ## Build and install to ~/go/bin and hook into shell rc files (POSIX shells; use install.ps1 on Windows)
ifeq ($(OS),Windows_NT)
	@echo On Windows, use: powershell -ExecutionPolicy Bypass -File install.ps1
else
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
endif

build-windows: ## Cross-compile pw.exe for Windows (amd64)
ifeq ($(OS),Windows_NT)
	cmd /c set GOOS=windows&& set GOARCH=amd64&& go build -o pw.exe .
else
	GOOS=windows GOARCH=amd64 go build -o pw.exe .
endif

test: ## Run all tests
ifeq ($(OS),Windows_NT)
	cmd /c go test ./...
else
	go test ./...
endif

clean: ## Remove built binaries
ifeq ($(OS),Windows_NT)
	-del /q pw.exe 2>nul
	-del /q pw 2>nul
else
	rm -f pw pw.exe
endif
