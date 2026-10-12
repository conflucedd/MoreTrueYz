GO  ?= go
BIN := bin
CMDS := moretrueyz yaz0verify yaz0rev yaz0tok

.PHONY: all build test vet fmt clean

all: build

build:
	@mkdir -p $(BIN)
	@for c in $(CMDS); do \
		echo "  build $$c"; \
		$(GO) build -o $(BIN)/$$c ./cmd/$$c || exit 1; \
	done

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')

clean:
	rm -rf $(BIN)
