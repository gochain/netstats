
# Build in a stock Go builder container
FROM golang:1.23-alpine AS builder
LABEL org.opencontainers.image.source https://github.com/gochain/netstats

RUN apk --no-cache add build-base git gcc linux-headers npm

ENV GENESIS_VERSION=0.2.1

ENV D=/src/netstats
WORKDIR $D

RUN wget https://github.com/benbjohnson/genesis/releases/download/v0.2.1/genesis-v0.2.1-linux-amd64.tar.gz && ls \
	&& tar zxvf genesis-v0.2.1-linux-amd64.tar.gz

# cache dependencies
ADD go.mod $D
ADD go.sum $D
RUN go mod download
# build
ADD . $D

RUN npm install -g grunt-cli
RUN npm install && grunt && grunt build

# RUN make generate
# RUN mkdir -p dist
RUN cd dist && ../genesis -pkg assets -o ../assets/assets.gen.go index.html css fonts images js
RUN ls -al assets
RUN go build -o /tmp/netstats ./cmd/netstats

# Pull all binaries into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /tmp/netstats /usr/local/bin/netstats

WORKDIR /netstats

CMD ["netstats", "-strict"]
