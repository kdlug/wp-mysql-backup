# Step 1 - build an executable binary
FROM golang:alpine as builder

ENV SERVICE=wp-mysql-backup
ENV BUILD_DIR=$GOPATH/src/github.com/kdlug/${SERVICE}/

RUN apk update && apk add --no-cache --upgrade \
    git
WORKDIR $BUILD_DIR
COPY main.go Gopkg.toml Gopkg.lock ./
RUN go get -u github.com/golang/dep/cmd/dep && dep ensure -vendor-only
RUN go build -o $GOPATH/bin/${SERVICE} .

# Step 2 - build an image
FROM alpine:3.8
RUN apk update && apk add --no-cache --upgrade \
    mysql-client \
    openssh-client \
    git && \
    mkdir -p /app && \
    mkdir -p /root/.ssh

# Add private key
COPY ssh/id_rsa /root/.ssh/id_rsa
RUN chmod 600 ~/.ssh/id_rsa
# RUN ssh-keyscan -H gitlab.com >> ~/.ssh/known_hosts

WORKDIR /app
COPY --from=builder /go/bin/${SERVICE} /app/${SERVICE}

#CMD ["tail", "-f", "/dev/null" ]
ENTRYPOINT ["./wp-mysql-backup"]
