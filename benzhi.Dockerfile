FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
ENV GOTOOLCHAIN=local
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build ./... && go build -o /app/keyproof ./cmd/keyproof

FROM docker.m.daocloud.io/library/alpine:3.20
RUN apk add --no-cache ca-certificates sqlite-libs
COPY --from=build /app/keyproof /app/keyproof
WORKDIR /data
ENTRYPOINT ["/app/keyproof"]
CMD ["--smoke-test"]
