# Build in a stock Go builder container
FROM golang:1.13-alpine as builder

RUN apk --no-cache add build-base git bzr mercurial gcc linux-headers npm
RUN npm install -g grunt-cli

ENV GENESIS_VERSION 0.2.1
RUN cd /usr/local/bin \
	&& wget https://github.com/benbjohnson/genesis/releases/download/v0.2.1/genesis-v0.2.1-linux-amd64.tar.gz && ls \
	&& tar zxvf genesis-v0.2.1-linux-amd64.tar.gz

ENV D=/src/netstats
WORKDIR $D
# cache dependencies
ADD go.mod $D
ADD go.sum $D
RUN go mod download
# build
ADD . $D
RUN npm install && grunt && grunt build
RUN make
RUN go build -o /tmp/netstats ./cmd/netstats

# Pull all binaries into a second stage deploy alpine container
FROM alpine:latest

RUN apk add --no-cache ca-certificates
COPY --from=builder /tmp/netstats /usr/local/bin/netstats

WORKDIR /netstats

CMD ["netstats", "-strict"]
