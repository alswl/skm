# Install directory override. If empty, auto-detect a writable directory in PATH.
# Auto-detection priority: /opt/homebrew/bin > HOME/.local/bin > /usr/local/bin > GOPATH/bin > HOME/go/bin
INSTALL_DIR ?=

.PHONY: install
install: build ## Install binaries to PATH
	@if [ -z "$(TARGETS)" ]; then                                                                      \
		echo "No targets to install. Define TARGETS in your Makefile.";                                   \
		exit 0;                                                                                           \
	fi;                                                                                                \
	_INSTALL_DIR="";                                                                                   \
	if [ -n "$(INSTALL_DIR)" ]; then                                                                   \
		_INSTALL_DIR="$(INSTALL_DIR)";                                                                    \
		if [ ! -d "$$_INSTALL_DIR" ]; then                                                                \
			mkdir -p "$$_INSTALL_DIR" || { echo "Error: cannot create directory $$_INSTALL_DIR"; exit 1; };  \
		fi;                                                                                               \
		if [ ! -w "$$_INSTALL_DIR" ]; then                                                                \
			echo "Error: $$_INSTALL_DIR is not writable. Try: sudo make install INSTALL_DIR=$$_INSTALL_DIR or choose another directory.";  \
			exit 1;                                                                                          \
		fi;                                                                                               \
	else                                                                                               \
		for _d in /opt/homebrew/bin $(HOME)/.local/bin /usr/local/bin $(GOPATH)/bin $(HOME)/go/bin; do    \
			if [ -d "$$_d" ] && [ -w "$$_d" ]; then                                                          \
				_INSTALL_DIR="$$_d";                                                                            \
				break;                                                                                          \
			fi;                                                                                              \
		done;                                                                                             \
		if [ -z "$$_INSTALL_DIR" ]; then                                                                  \
			echo "Error: no writable directory found in PATH candidates. Set INSTALL_DIR explicitly, e.g.:";  \
			echo "  make install INSTALL_DIR=$(HOME)/.local/bin";                                            \
			echo "  make install INSTALL_DIR=/usr/local/bin";                                                \
			exit 1;                                                                                          \
		fi;                                                                                               \
	fi;                                                                                                \
	_COUNT=0;                                                                                          \
	for _target in $(TARGETS); do                                                                      \
		if [ ! -f "$(OUTPUT_DIR)/$$_target" ]; then                                                       \
			echo "Error: $(OUTPUT_DIR)/$$_target not found. Run 'make build' first.";                        \
			exit 1;                                                                                          \
		fi;                                                                                               \
		cp "$(OUTPUT_DIR)/$$_target" "$$_INSTALL_DIR/$$_target" || { echo "Error: failed to install $$_target to $$_INSTALL_DIR"; exit 1; };  \
		chmod +x "$$_INSTALL_DIR/$$_target" || { echo "Error: failed to set executable permission on $$_INSTALL_DIR/$$_target"; exit 1; };  \
		echo "Installed $$_target -> $$_INSTALL_DIR/$$_target";                                           \
		_COUNT=$$((_COUNT + 1));                                                                          \
	done;                                                                                              \
	if [ $$_COUNT -eq 1 ]; then                                                                        \
		echo "Installed 1 binary to $$_INSTALL_DIR";                                                      \
	else                                                                                               \
		echo "Installed $$_COUNT binaries to $$_INSTALL_DIR";                                             \
	fi;                                                                                                \
	case ":$${PATH}:" in                                                                               \
		*":$$_INSTALL_DIR:"*) ;;                                                                          \
		*) echo "Warning: $$_INSTALL_DIR is not in PATH. Add: export PATH=\"$$_INSTALL_DIR:\$$PATH\""; ;;  \
	esac;                                                                                              \
	for _target in $(TARGETS); do                                                                      \
		_WHICH=$$(command -v "$$_target" 2>/dev/null || true);                                            \
		if [ -n "$$_WHICH" ] && [ "$$_WHICH" != "$$_INSTALL_DIR/$$_target" ]; then                        \
			echo "Warning: '$$_target' resolves to '$$_WHICH' which shadows the installed binary at $$_INSTALL_DIR/$$_target";  \
		fi;                                                                                               \
	done
