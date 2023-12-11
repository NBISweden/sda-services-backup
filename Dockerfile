FROM golang:1.21.5-alpine3.17
COPY . .
ENV GO111MODULE=on
ENV GOPATH=$PWD
ENV XDG_CACHE_HOME=/tmp/.cache
ENV CGO_ENABLED=0
ENV GOOS=linux
RUN go build -ldflags "-extldflags -static" -o backup-svc .

FROM alpine:3.18
RUN apk add --no-cache postgresql-client mongodb-tools
COPY --from=0 go/backup-svc /usr/local/bin/
USER 65534
