BIN  := bin/sapato
MOCK := $(CURDIR)/.work/mock

# Extra CLI args for `make run`, e.g. make run ARGS="status --json"
ARGS ?=

.PHONY: build run test mock mock-reset clean

build:
	go build -o $(BIN) ./cmd/sapato

test:
	go test -count=1 ./...

# Disposable copy of testdata: a vault, every agent's dir and a fake HOME,
# so a run never touches testdata/ or the real agent dirs.
mock: $(MOCK)/config.toml

$(MOCK)/config.toml:
	mkdir -p $(MOCK)/home $(MOCK)/targets/pi $(MOCK)/targets/opencode
	cp -a testdata/skill-db $(MOCK)/skill-db
	cp -a testdata/targets/. $(MOCK)/targets/
	printf '%s\n' \
		'vault = "$(MOCK)/skill-db"' \
		'[agents]' \
		'claude   = "$(MOCK)/targets/claude"' \
		'cursor   = "$(MOCK)/targets/cursor"' \
		'pi       = "$(MOCK)/targets/pi"' \
		'opencode = "$(MOCK)/targets/opencode"' \
		'agents   = "$(MOCK)/targets/agents"' \
		> $@

mock-reset:
	rm -rf $(MOCK)
	$(MAKE) mock

run: build mock
	HOME=$(MOCK)/home ./$(BIN) --config $(MOCK)/config.toml $(ARGS)

clean:
	rm -rf bin $(MOCK)
