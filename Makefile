test:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf coverage.out coverage.html

.PHONY: test clean
