default: generate

docker: 
	docker build --progress=plain -t gochain/netstats .
	docker tag gochain/netstats gcr.io/gochain-core/netstats:latest
	docker tag gochain/netstats ghcr.io/gochain/netstats:latest

docker-test:
	docker build --target builder -t gochain/netstats-builder .
	docker run gochain/netstats-builder go test ./...

run:
	docker run --rm -it -p 3000:3000 -e WS_SECRET=$(WS_SECRET) gochain/netstats

release: docker
	./release.sh

generate:
	mkdir -p dist
	cd dist && genesis -pkg assets -o ../assets/assets.gen.go index.html css fonts images js

.PHONY: default generate test build docker docker-test release run
