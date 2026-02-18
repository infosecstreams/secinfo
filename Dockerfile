FROM golang:1.26-trixie AS builder

COPY go.mod go.sum /build/
COPY secinfo.go /build/
COPY streamers /build/streamers

WORKDIR /build

RUN apt update && apt install -y upx
RUN CGO_ENABLED=0 GOAMD64=v3 go build -v -ldflags '-s -w -extldflags "-static"' -tags 'osusergo,netgo,static' -asmflags 'all=-trimpath={{.Env.GOPATH}}' .
RUN upx --ultra-brute secinfo && upx -t secinfo

FROM scratch

WORKDIR /app

COPY --from=builder /build/secinfo .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

CMD ["/app/secinfo"]
LABEL org.opencontainers.image.authors='goproslowyo@gmail.com'
LABEL org.opencontainers.image.description="Update Markdown Files based on Streaming Activity "
LABEL org.opencontainers.image.licenses='Apache-2.0'
LABEL org.opencontainers.image.source='https://github.com/infosecstreams/secinfo'
LABEL org.opencontainers.image.url='https://infosecstreams.com'
LABEL org.opencontainers.image.vendor='InfoSec Streams'
