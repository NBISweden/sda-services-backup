FROM golang:1.15.6-alpine3.12
RUN apk add --no-cache git coreutils && rm -rf /var/cache/apk/*
COPY . .
ENV GO111MODULE=on
ENV GOPATH=$PWD
ENV XDG_CACHE_HOME=/tmp/.cache
ENV CGO_ENABLED=0
ENV GOOS=linux
RUN go build -ldflags "-extldflags -static" -o svc .
RUN echo "nobody:x:65534:65534:nobody:/:/sbin/nologin" > passwd

USER 65534
EXPOSE 8080
