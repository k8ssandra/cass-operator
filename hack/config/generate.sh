#!/usr/bin/env bash
OUTPUTFILE=../../apis/cassandra/v1beta1/cassandra_config_generated.go
curl -sL https://raw.githubusercontent.com/apache/cassandra/cassandra-4.0.6/src/java/org/apache/cassandra/config/Config.java --output Config-406.java
curl -sL https://raw.githubusercontent.com/apache/cassandra/cassandra-4.0.0/src/java/org/apache/cassandra/config/Config.java --output Config-400.java
curl -sL https://raw.githubusercontent.com/apache/cassandra/cassandra-4.0/src/java/org/apache/cassandra/config/Config.java --output Config-40.java
curl -sL https://raw.githubusercontent.com/apache/cassandra/cassandra-3.11/src/java/org/apache/cassandra/config/Config.java --output Config-311.java
curl -sL https://raw.githubusercontent.com/apache/cassandra/trunk/src/java/org/apache/cassandra/config/Config.java --output Config-trunk.java
pipenv run python parse.py > $OUTPUTFILE
go fmt $OUTPUTFILE
rm -f *.java