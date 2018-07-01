FROM golang:1.10-stretch
RUN wget http://git.io/trans && mv trans /usr/bin/ && chmod +x /usr/bin/trans && apt update && apt install -y gawk curl
WORKDIR /go/src/app
COPY cmd/. .
RUN go get -d -v ./...
RUN go install -v ./...

CMD ["translate"]