FROM golang:1.10-stretch
RUN apt update &&  apt install -y ca-certificates gawk curl && curl -k -L -o trans https://git.io/trans && mv trans /usr/bin/ && chmod +x /usr/bin/trans
WORKDIR /go/src/app
COPY cmd/. .
RUN go get -d -v ./...
RUN go install -v ./...

CMD ["translate"]