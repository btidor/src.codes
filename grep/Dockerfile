FROM golang:1.17 as build

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

FROM alpine:latest

WORKDIR /
COPY --from=build /go/bin/grep /grep

ENTRYPOINT ["/grep"]
