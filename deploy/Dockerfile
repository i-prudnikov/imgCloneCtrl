FROM golang:1.13-alpine3.12 as build

RUN mkdir -p /imgCloneCtrl
WORKDIR /imgCloneCtrl

ADD . /imgCloneCtrl

RUN commit=$(git rev-parse HEAD 2>/dev/null || echo "unknown") \
 && version=$(git describe --contains "$commit" 2>/dev/null || echo "unknown") \
 && branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown") \
 && go mod vendor \
 && go build -ldflags "-X main.version=${version} -X main.branch=${branch} -X main.commit=${commit} -s -w -extldflags '-static'" -o imgCloneCtrl github.com/i-prudnikov/imgCloneCtrl

FROM alpine:3.12
COPY --from=build /imgCloneCtrl/imgCloneCtrl /usr/local/bin/imgCloneCtrl
RUN apk add --no-cache tini
# Tini is now available at /sbin/tini
ENTRYPOINT ["/sbin/tini", "--"]