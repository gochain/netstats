

docker: 
	docker build -t gcr.io/gochain-core/netstats:latest .

run:
	docker run --rm -it -p 3000:3000 -e WS_SECRET=MYSECRET gcr.io/gochain-core/netstats

release: docker
	./release.sh

.PHONY: test build docker release run
