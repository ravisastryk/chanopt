.PHONY: build test bench bench-compare lint demo install clean

build:
	go build -o bin/chanopt ./cmd/chanopt

test:
	go test -race -count=1 ./...

bench:
	cd demos && go test -bench=. -benchmem -count=5 -timeout=120s | tee bench.txt

bench-compare:
	@echo "── Channel vs Optimized (side-by-side) ──"
	cd demos && go test -bench=. -benchmem -count=5 -timeout=120s | \
		grep -E 'Benchmark|^$$' | column -t

lint: build
	go vet ./...
	cd demos && go vet -vettool=../bin/chanopt ./antipatterns/ 2>&1 || true

demo: build
	@echo "── Running chanopt on demo anti-patterns ──"
	cd demos && go vet -vettool=../bin/chanopt ./antipatterns/ 2>&1 || true

install:
	go install ./cmd/chanopt

clean:
	rm -rf bin/ demos/bench.txt
