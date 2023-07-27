FROM redhat/ubi8:latest as builder
ARG VERSION
ARG TARGETPLATFORM

# Install Vector
ENV VECTOR_VERSION 0.31.0
RUN case ${TARGETPLATFORM} in \
         "linux/amd64")  VECTOR_ARCH=x86_64  ;; \
         "linux/arm64")  VECTOR_ARCH=aarch64  ;; \
    esac \
 && rpm -i https://packages.timber.io/vector/${VECTOR_VERSION}/vector-${VECTOR_VERSION}-1.${VECTOR_ARCH}.rpm

FROM redhat/ubi8-micro:latest

ARG VERSION
ARG TARGETPLATFORM

LABEL maintainer="DataStax, Inc <info@datastax.com>"
LABEL name="system-logger"
LABEL vendor="DataStax, Inc"
LABEL release="${VERSION}"
LABEL version="${VERSION}"
LABEL summary="Sidecar for DataStax Kubernetes Operator for Apache Cassandra "
LABEL description="Sidecar to output Cassandra system logs to stdout"

# Copy our configuration
COPY ./config/logger/vector_config.toml /etc/vector/vector.toml
COPY --from=builder /usr/lib64/libstdc++.so.6 /usr/lib64/libstdc++.so.6
COPY --from=builder /usr/bin/vector /usr/bin/vector

ADD https://raw.githubusercontent.com/vectordotdev/vector/master/LICENSE /licences/LICENSE
COPY ./LICENSE.txt /licenses/

# Non-root user, cassandra as default
USER cassandra
ENTRYPOINT ["/usr/bin/vector"]
