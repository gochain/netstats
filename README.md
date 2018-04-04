GoChain Network Stats
============
[![CircleCI](https://circleci.com/gh/gochain-io/netstats/tree/master.svg?style=svg)](https://circleci.com/gh/gochain-io/netstats/tree/master)

This is a visual interface for tracking GoChain network status. It uses WebSockets to receive stats from running nodes and output them through an angular interface. It is the front-end implementation for [net-intelligence-api](https://github.com/gochain-io/net-intelligence-api).

![Screenshot](screenshot.png?raw=true)

## Prequisites

* Docker

## Run

```sh
export WS_SECRET=YOURSECRET
make docker
WS_SECRET=YOOOOO make run
```

Check it out at http://localhost:3000

# Running outside Docker

## Prerequisite
* node
* npm

## Installation
Make sure you have node.js and npm installed.

Clone the repository and install the dependencies

```bash
git clone https://github.com/gochain-io/netstats
cd netstats
npm install
sudo npm install -g grunt-cli
```

##Build the resources
NetStats features two versions: the full version and the lite version. In order to build the static files you have to run grunt tasks which will generate dist or dist-lite directories containing the js and css files, fonts and images.


To build the full version run
```bash
grunt
```

To build the lite version run
```bash
grunt lite
```

If you want to build both versions run
```bash
grunt all
```

##Run

```bash
npm start
```

see the interface at http://localhost:3000
