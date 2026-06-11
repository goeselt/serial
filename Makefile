.PHONY: build check clean release-assets

BINARY := serial
DIST_DIR := dist
RELEASE_GOOS := linux
RELEASE_GOARCH := amd64 arm64

build:
	go build ./...

check:
	gofmt -l .
	go vet ./...
	go test -race ./...

clean:
	rm -rf $(DIST_DIR)

release-assets: clean
	mkdir -p $(DIST_DIR)
	@for goarch in $(RELEASE_GOARCH); do \
		artifact="$(BINARY)_$(RELEASE_GOOS)_$${goarch}"; \
		package_dir="$(DIST_DIR)/$${artifact}"; \
		mkdir -p "$$package_dir"; \
		GOOS=$(RELEASE_GOOS) GOARCH="$$goarch" CGO_ENABLED=0 \
			go build -trimpath -ldflags='-s -w' -o "$$package_dir/$(BINARY)" .; \
		cp LICENSE README.md "$$package_dir/"; \
		tar -C "$$package_dir" -czf "$(DIST_DIR)/$${artifact}.tar.gz" .; \
		rm -rf "$$package_dir"; \
	done
	cd $(DIST_DIR) && sha256sum *.tar.gz > checksums.txt
