default: generate

docker: 
	docker build -t gcr.io/gochain-core/netstats:latest .

docker-test:
	docker build --target builder -t gochain/netstats-builder .
	docker run gochain/netstats-builder go test ./...

run:
	docker run --rm -it -p 3000:3000 -e WS_SECRET=$(WS_SECRET) gcr.io/gochain-core/netstats

release: docker
	./release.sh

generate:
	cd dist && genesis -pkg assets -o ../assets/assets.gen.go index.html css fonts images js

.PHONY: default generate test build docker docker-test release run
