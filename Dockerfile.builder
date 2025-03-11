FROM golang:1.23 as builder

RUN mkdir -p /hej/
WORKDIR /hej



ENV OS=linux
ENV ARCH=amd64
ARG GIT_SHA
ARG VERSION
ARG DATE

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 GOOS=${OS} GOARCH=${ARCH} go build -ldflags "-s -w \
    -X github.com/huhenry/hej/pkg/version.commitSHA=${GIT_SHA} \
    -X github.com/huhenry/hej/pkg/version.latestVersion=${VERSION} \
    -X github.com/huhenry/hej/pkg/version.date=${DATE}" \
    -a -o bin/hej cmd/hej/main.go



FROM alpine:3.19


RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk --no-cache --update add curl ca-certificates

WORKDIR /

COPY --from=builder /hej/bin/hej .

ENTRYPOINT ["./hej"]