FROM golang:1.21-alpine

EXPOSE 4000
WORKDIR /app

RUN CGO_ENABLED=0 go install -ldflags "-s -w -extldflags '-static'" \
    github.com/go-delve/delve/cmd/dlv@latest
RUN go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

ENTRYPOINT [ \
    "dlv", \
    "debug", \
    "--listen=:4000", \
    "--headless", \
    "--log", \
    "--api-version=2", \
    "--accept-multiclient", \
    "--continue" \
    ]
