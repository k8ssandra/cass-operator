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
--destination-topic-name public/default/data-ks1.testtable \
--parallelism 1 \
--source-config '{
    "events.topic": "persistent://public/default/events-cdc-ks1.testtable",
    "keyspace": "ks1",
    "table": "testtable",
    "datastax-java-driver.basic.contact-points": "test-cluster-dc1-all-pods-service.cass-operator.svc.cluster.local:9042",
    "loadBalancing.localDc": "dc1",
    "auth.provider": "None",
    "batch.size": 1
    }';
    return $?
}
until source_exists; do sleep 10; create_source; done