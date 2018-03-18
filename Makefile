

docker: 
	docker build -t gochain/netstats:latest .

run:
	docker run --rm -it -p 3000:3000 -e WS_SECRET=MYSECRET gochain/netstats

release: docker
	./release.sh

.PHONY: test build docker release run
