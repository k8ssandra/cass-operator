#!/usr/bin/env fish
set VERSIONS 1.0.0 1.1.0 1.2.0 1.3.0 1.4.0 1.4.1 1.5.0 1.5.1 1.6.0 1.7.0 1.7.1
set LATEST_VERSION $VERSIONS[-1..-1]
for VERSION in $VERSIONS
  echo "Version: $VERSION"
  echo "docker build -t docker.io/k8ssandra/cass-operator-bundle:$VERSION -f bundle-$VERSION.Dockerfile ."
  docker build -t docker.io/k8ssandra/cass-operator-bundle:$VERSION -f bundle-$VERSION.Dockerfile .
  docker push docker.io/k8ssandra/cass-operator-bundle:$VERSION
end
docker tag docker.io/k8ssandra/cass-operator-bundle:$LATEST_VERSION docker.io/k8ssandra/cass-operator-bundle:latest
docker push docker.io/k8ssandra/cass-operator-bundle:latest

set BUNDLELIST ""

for VERSION in $VERSIONS
  set BUNDLELIST $BUNDLELIST,docker.io/k8ssandra/cass-operator-bundle:$VERSION
end
# Remove ',' from start of bundlelist
set BUNDLELIST (string sub -s 2 $BUNDLELIST)

echo "opm index add --bundles $BUNDLELIST --tag docker.io/k8ssandra/cass-operator-index:latest -u docker"
opm index add --bundles $BUNDLELIST --tag docker.io/k8ssandra/cass-operator-index:$LATEST_VERSION -u docker
docker tag docker.io/k8ssandra/cass-operator-index:$LATEST_VERSION docker.io/k8ssandra/cass-operator-index:latest
docker push docker.io/k8ssandra/cass-operator-index:$LATEST_VERSION
docker push docker.io/k8ssandra/cass-operator-index:latest
