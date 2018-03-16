FROM node:alpine

WORKDIR /netstats

RUN npm install -g grunt-cli
ADD package.* /netstats
RUN cd /netstats && npm install

ADD . /netstats
RUN	grunt && grunt build

CMD ["npm", "start"]
