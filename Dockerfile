FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /nexagate ./cmd/nexagate

FROM alpine:3.22
RUN addgroup -S nexagate && adduser -S -G nexagate nexagate
COPY --from=build /nexagate /usr/local/bin/nexagate
USER nexagate
EXPOSE 9080
ENTRYPOINT ["/usr/local/bin/nexagate"]
CMD ["serve", "--config", "/var/lib/nexagate/panel.json"]
