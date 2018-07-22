FROM golang:1.10-stretch
RUN apt update && apt install -y gawk curl git hunspell\
    $(apt search hunspell- | grep '/' | tr '/' ' ' | awk '{print $1}' | grep -v lib | grep -v 'frami' | grep -v 'modern' | grep -v 'comprehensive' | grep -v 'revised' | grep -v 'gl-es' | grep -v 'sv')\
     bsdmainutils \
     aspell-\*
RUN curl -L -o trans https://git.io/trans && mv trans /usr/bin/ && chmod +x /usr/bin/trans
WORKDIR /go/src/app
COPY cmd/. .
RUN go get -d -v ./...
RUN go install -v ./...

CMD ["translate"]