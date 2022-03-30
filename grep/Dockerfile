FROM golang:1.18 as build
ARG COMMIT

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -v -o /go/build/ -ldflags "-X main.commit=${COMMIT}" ./...

FROM alpine:latest

WORKDIR /var/src.codes
COPY --from=build /go/build/ .
COPY distributions.toml .
