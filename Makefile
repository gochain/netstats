default: generate

docker: 
	docker build -t gcr.io/gochain-core/netstats:latest .

run:
	docker run --rm -it -p 3000:3000 -e WS_SECRET=$(WS_SECRET) gcr.io/gochain-core/netstats

release: docker
	./release.sh

generate: assets/assets.gen.go

assets/assets.gen.go: $(shell find dist/ -type f)
	cd dist && genesis -pkg assets -o ../assets/assets.gen.go index.html favicon.ico css fonts js

.PHONY: default generate test build docker release run
