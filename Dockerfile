# Copy the controller-manager into a thin image
FROM alpine:3.19

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories \
    && apk --no-cache --update add curl ca-certificates
    
WORKDIR /
COPY ./bin/hej .
ENTRYPOINT ["./hej"]