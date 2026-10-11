.PHONY: all agent probe size clean

all: probe

agent:
	@mkdir -p bin
	@if [ -f agent/cmd/sds-agent/main.go ]; then \
		cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sds-agent ./cmd/sds-agent; \
	else \
		echo "not implemented yet"; \
	fi

probe:
	@mkdir -p bin
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/pemprobe ./cmd/pemprobe
	cd agent && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o ../bin/sizeprobe ./cmd/sizeprobe

size: probe
	@mkdir -p bin
	cp bin/sizeprobe bin/sizeprobe.upx
	upx --best --lzma -f bin/sizeprobe.upx
	./scripts/check-size.sh bin/sizeprobe.upx 4500000

clean:
	rm -rf bin .test-certs
