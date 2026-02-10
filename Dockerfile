# Default to Go 1.24
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-alpine as build

# Necessary to run 'go get' and to compile the linked binary
RUN apk add git musl-dev mailcap

WORKDIR /go/src/github.com/sooua/send.to

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# build & install server
RUN CGO_ENABLED=0 go build -tags netgo -ldflags "-X github.com/sooua/send.to/cmd.Version=$(git describe --tags) -a -s -w -extldflags '-static'" -o /go/bin/sendto

# build a minimal healthcheck binary for scratch image
RUN printf 'package main\nimport("net/http";"os")\nfunc main(){r,e:=http.Get("http://localhost:8080/health.html");if e!=nil||r.StatusCode!=200{os.Exit(1)}}\n' > /tmp/healthcheck.go && \
    CGO_ENABLED=0 go build -ldflags "-s -w" -o /go/bin/healthcheck /tmp/healthcheck.go

ARG PUID=5000 \
    PGID=5000 \
    RUNAS

RUN mkdir -p /tmp/useradd /tmp/empty && \
    if [ ! -z "$RUNAS" ]; then \
    echo "${RUNAS}:x:${PUID}:${PGID}::/nonexistent:/sbin/nologin" >> /tmp/useradd/passwd && \
    echo "${RUNAS}:!:::::::" >> /tmp/useradd/shadow && \
    echo "${RUNAS}:x:${PGID}:" >> /tmp/useradd/group && \
    echo "${RUNAS}:!::" >> /tmp/useradd/groupshadow; else touch /tmp/useradd/unused; fi

FROM scratch AS final
LABEL maintainer="sooua"
ARG RUNAS

COPY --from=build /etc/mime.types /etc/mime.types
COPY --from=build /tmp/empty /tmp
COPY --from=build /tmp/useradd/* /etc/
COPY --from=build --chown=${RUNAS}  /go/bin/sendto /go/bin/sendto
COPY --from=build /go/bin/healthcheck /go/bin/healthcheck
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER ${RUNAS}

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/go/bin/healthcheck"]

ENTRYPOINT ["/go/bin/sendto", "--listener", ":8080"]

EXPOSE 8080
