FROM alpine:3.13

# Add Tini
RUN apk add --no-cache tini
ENTRYPOINT ["/sbin/tini", "--"]

# Run your program under Tini
CMD ["tail", "-n+1", "-F", "/var/log/cassandra/system.log"]
