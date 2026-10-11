.PHONY: all agent server run-server probe size seed clean test ui ui-dev

all: agent server

ui:
	cd ui && npm run build

ui-dev:
	cd ui && npm run dev

agent:
	@mkdir -p bin
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sds-agent ./cmd/sds-agent
	upx --best --lzma -f bin/sds-agent
	./scripts/check-size.sh bin/sds-agent 4500000

server: ui
	@mkdir -p bin
	cd api && CGO_ENABLED=1 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sds-server ./cmd/sds-server

run-server: server
	./bin/sds-server

probe:
	@mkdir -p bin
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/pemprobe ./cmd/pemprobe
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sizeprobe ./cmd/sizeprobe

size: probe
	@mkdir -p bin
	cp bin/sizeprobe bin/sizeprobe.upx
	upx --best --lzma -f bin/sizeprobe.upx
	./scripts/check-size.sh bin/sizeprobe.upx 4500000

seed:
	@mkdir -p .test-certs
	cd agent && go run ./cmd/seedcerts ../.test-certs/corpus

test:
	cd agent && go test -v ./...
	cd api && go test -v ./...

clean:
	rm -rf bin .test-certs data/*.duckdb*
