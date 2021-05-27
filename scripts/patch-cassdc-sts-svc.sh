#!/bin/bash
# This script is a work around for 
# https://github.com/k8ssandra/cass-operator/issues/103. cass-operator 1.7.0
# changes the name of the serviceName property in the StatefulSet spec. This
# will break existing installs since the serviceName property is immutable. The
# script deletes the CassandraDatacenter and StatefulSet without also deleting
# the Cassandra pods. The script then recreates the CassandraDatacenter which
# will then allow the StatefulSet to be recreated with the new serviceName. 
# There is no downtime when applying this patch. Cassandra pods remain up.
#
# Note that the script assumes that the namespace of the current context is the
# namespace in which the CassandraDatacenter is deployed.
#
# The script uses jq (https://stedolan.github.io/jq/) for json queries.
#
# The script accepts 3 space-separated options:
#
#   --operator           The name of the cass-operator deployment. Required.
#   --datacenter         The name of the CassandraDatacenter. Required.
#   --operator-namespace The namespace in which cass-operator is running. Optional.

set -e

scale_down_cass_operator() {
  kubectl $operator_ns scale deployment $cass_operator --replicas 0

  replicas=`kubectl get deployment $cass_operator -o json | jq -r '.status.readyReplicas'`
  while [ "$replicas" != "null" ] && [ $replicas -ne 0 ]
  do
    echo "Waiting for cass-operator scale down to complete"
    sleep 1
    replicas=`kubectl get deployment $cass_operator -o json | jq -r '.status.readyReplicas'`
  done
  echo "cass-operator is scaled down to 0 replicas"
}

delete_objects() {
  cluster=`kubectl get cassdc $dc -o json | jq -r '.spec.clusterName'`

  echo "Removing finalizer from CassandraDatacenter $dc"
  kubectl patch cassdc $dc --type=merge --patch '{"metadata": {"finalizers": []}}'

  echo "Deleting CassandraDatacenter $dc"
  kubectl delete --cascade=$cascade_mode --wait=true cassdc $dc

  echo "Deleting StatefulSets"
  kubectl delete sts --cascade=$cascade_mode --wait=true -l cassandra.datastax.com/datacenter=$dc,cassandra.datastax.com/cluster=$cluster
}

restore_objects() {
  echo "Scaling cass-operator back up"
  kubectl $operator_ns scale deployment $cass_operator --replicas 1

  echo "Recreating CassandraDatacenter $dc"
  echo $dc_copy | kubectl apply -f -
}

################
# start script #
################

# This should be the name of the cass-operator Deployment.
cass_operator=""

# This should be the name of the CassandraDatacenter.
dc=""

# This is optional. It should be set if cass-operator is deployed in a 
# different than the CassandraDatacenter which would be common when the 
# operator is configured to watch multiple namespaces.
operator_ns=""

client_major_version=`kubectl version --client -o json | jq -r '.clientVersion.major'`

client_minor_version=`kubectl version --client -o json | jq -r '.clientVersion.minor'`

cascade_mode="orphan"

while [[ $# -gt 0  ]]
do
  arg=$1
  case $arg in
    --operator)
    cass_operator="$2"
    shift
    shift
    ;;
    --datacenter)
    dc="$2"
    shift
    shift
    ;;
    --operator-namespace)
    operator_ns="-n $2"
    shift
    shift
    ;;
    *)
    shift
    ;;
  esac
done

if [ -z $cass_operator ]; then
  echo "The --operator option is required and should specify the name of the cass-operator deployment"
  exit 1
fi

if [ -z $dc ]; then
  echo "The --datacenter option is required and should specify the name of the cassandradatacenter"
fi 

echo "Detected kubectl client version v${client_major_version}.${client_minor_version}"
if [ $client_major_version -eq 1 ]; then
  # The check for versions under 1.16 is arbitrary and only chose to match the general support in K8ssandra
  if [ $client_minor_version -lt 16 ]; then
    echo "The detected kubectl client version is not supported, it must be >= v1.16"
    exit 1
  fi
  if [ $client_minor_version -lt 20 ]; then
    cascade_mode="false"
  fi
fi
echo "Will use delete --cascade=${cascade_mode}"

# Store a copy of the CassandraDatacenter object to recreate it later.
dc_copy=`kubectl get cassdc $dc -o json`

scale_down_cass_operator

delete_objects

restore_objects
