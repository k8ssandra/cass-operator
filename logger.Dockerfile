FROM redhat/ubi8-micro:latest

ARG VERSION
# Default the tini download to amd64, but allow for overide to a different arch at build time
ARG TINI_BIN=tini

LABEL maintainer="DataStax, Inc <info@datastax.com>"
LABEL name="system-logger"
LABEL vendor="DataStax, Inc"
LABEL release="${VERSION}"
LABEL version="${VERSION}"
LABEL summary="Sidecar for DataStax Kubernetes Operator for Apache Cassandra "
LABEL description="Sidecar to output Cassandra system logs to stdout"

# Add Tini
ENV TINI_VERSION v0.19.0
ADD https://github.com/krallin/tini/releases/download/${TINI_VERSION}/${TINI_BIN} /sbin/tini
ADD https://raw.githubusercontent.com/krallin/tini/master/LICENSE /licenses/LICENSE
RUN chmod +x /sbin/tini
COPY ./LICENSE.txt /licenses/

# Non-root user, cassandra as default
USER cassandra:cassandra
ENTRYPOINT ["/sbin/tini", "--"]

# Run your program under Tini
CMD ["tail", "-n+1", "-F", "/var/log/cassandra/system.log"]
