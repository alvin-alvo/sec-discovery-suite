.PHONY: all agent probe size seed clean test

all: agent

agent:
	@mkdir -p bin
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sds-agent ./cmd/sds-agent
	upx --best --lzma -f bin/sds-agent
	./scripts/check-size.sh bin/sds-agent 4500000

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

clean:
	rm -rf bin .test-certs
