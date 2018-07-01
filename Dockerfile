FROM golang:1.10-alpine
RUN apk --no-cache add ca-certificates gawk curl git bash
RUN curl -L -o trans https://git.io/trans && mv trans /usr/bin/ && chmod +x /usr/bin/trans
WORKDIR /go/src/app
COPY cmd/. .
RUN go get -d -v ./...
RUN go install -v ./...

CMD ["translate"]