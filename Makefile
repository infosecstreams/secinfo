VERSION=$$(git describe --tags)

docs:
	godoc -index -notes -play -timestamps -http 127.0.0.1:8000;

test:
	(go test -v -race -coverprofile=coverage.out ./...; \
	go tool cover -html=coverage.out -o coverage.html)

cover: test

clean:
	rm -rf coverage.out coverage.html
	rm -rf secinfo secinfo.test
	rm -rf dist

build:
	@echo ${VERSION}
	CGO_ENABLED=0 GOAMD64=v4 go build -v -trimpath -tags 'osusergo,netgo,static' -ldflags '-s -w -extldflags "-static"' .

docker-build:
	docker build -t secinfo:${VERSION} .

release:
	goreleaser release --snapshot --rm-dist

docker-run:
	docker run -it -v ${PWD}/templates:/app/templates -v ${PWD}/index.md:/app/index.md -v ${PWD}/streamers.csv:/app/streamers.csv secinfo:${VERSION}

docker-push:
	docker tag secinfo:${VERSION} ghcr.io/goproslowyo/secinfo:${VERSION}
	docker tag secinfo:${VERSION} ghcr.io/goproslowyo/secinfo:latest
	docker push ghcr.io/goproslowyo/secinfo:${VERSION}
	docker push ghcr.io/goproslowyo/secinfo:latest

.PHONY: clean test
