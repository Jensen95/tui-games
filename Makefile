GO ?= go
BINARY := lig
LIG_SEEDS ?= 250

.PHONY: all build test race cover vet fmt-check depguard lint fuzz bench corpus nightly clean

all: lint test build

build:
	$(GO) build -o $(BINARY) ./cmd/lig

test:
	LIG_SEEDS=$(LIG_SEEDS) $(GO) test ./...

race:
	LIG_SEEDS=$(LIG_SEEDS) $(GO) test -race ./...

cover:
	LIG_SEEDS=$(LIG_SEEDS) $(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	$(GO) tool cover -func=coverage.out | tail -1

vet:
	$(GO) vet ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed on:"; echo "$$out"; exit 1; fi

depguard:
	./scripts/depguard.sh

lint: fmt-check vet depguard

fuzz:
	./scripts/fuzz-all.sh

bench:
	$(GO) test -run '^$$' -bench . -benchmem ./...

# Build a de-duplicated puzzle corpus per game (Phase 3 wires cross-run dedup).
corpus: build
	@mkdir -p corpus
	@for g in $$(./$(BINARY) games | awk '{print $$1}'); do \
		[ "$$g" = "no" ] && continue; \
		./$(BINARY) generate --game $$g --count 25 --out corpus/$$g; \
	done

# Nightly: heavier property-test seed counts + fuzzing.
nightly:
	LIG_SEEDS=5000 $(GO) test -race -timeout 45m ./...
	FUZZTIME=60s ./scripts/fuzz-all.sh

clean:
	rm -f $(BINARY) coverage.out coverage.html
