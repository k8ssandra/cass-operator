#!/bin/bash

source_exists(){
  list=$(/pulsar/bin/pulsar-admin source list | grep -o "cassandra-source-1")
  if [[ -n $list ]];
  then
    return 0
  else
    return 1
  fi
}

create_source() {
/pulsar/bin/pulsar-admin source create \
--name cassandra-source-1 \
--source-type cassandra-source \
--tenant public \
--namespace default \
--destination-topic-name public/default/data-ks1.table1 \
--parallelism 1 \
--source-config '{
    "events.topic": "persistent://public/default/events-cdc-ks1.table1",
    "keyspace": "ks1",
    "table": "table1",
    "contactPoints": "localhost",
    "port": 9042,
    "loadBalancing.localDc": "DC1",
    "auth.provider": "PLAIN",
    "auth.username": "testuser",
    "auth.password": "testpass"
    }';
    return $?
}
until source_exists; do sleep 10; create_source; done