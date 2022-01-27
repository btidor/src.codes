FROM golang:1.17 as build

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -v -o /go/build/ ./...

FROM alpine:latest

WORKDIR /
COPY --from=build /go/build/grep /grep

ENTRYPOINT ["/grep"]
