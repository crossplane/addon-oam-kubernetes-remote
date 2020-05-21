FROM BASEIMAGE
RUN apk --no-cache add ca-certificates bash

ARG ARCH
ARG TINI_VERSION

ADD addon /usr/local/bin/crossplane-addon-oam-kubernetes-remote

COPY stack-package /

EXPOSE 8080
USER 1001
ENTRYPOINT ["crossplane-addon-oam-kubernetes-remote"]
