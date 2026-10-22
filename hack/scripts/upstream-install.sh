#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

set -e

_IMAGE_NAME="cmb-custom-registry"
_WEB_REPO="https://quay.io/repository/stolostron/${_IMAGE_NAME}?tab=tags"
_REPO="quay.io/stolostron/${_IMAGE_NAME}"

# This is needed for the deploy
echo "* Testing connection"
HOST_URL=`oc -n openshift-console get routes console -o jsonpath='{.status.ingress[0].routerCanonicalHostname}'`
if [ $? -ne 0 ]; then
    echo "ERROR: Make sure you are logged into an OpenShift Container Platform before running this script"
    exit 2
fi
#Shorten to the basedomain
HOST_URL=${HOST_URL/apps./}
echo "* Using baseDomain: ${HOST_URL}"
VER=`oc version | grep "Client Version:"`
echo "* oc CLI ${VER}"

printf "Find snapshot tags @ ${_WEB_REPO}\nEnter SNAPSHOT TAG: \n"
read -e -r SNAPSHOT_CHOICE

if [[ ! -n "${SNAPSHOT_CHOICE}" ]]; then
    echo "ERROR: Make sure you are provide a valid SNAPSHOT"
    exit 1
else 
    echo "SNAPSHOT_CHOICE is set to ${SNAPSHOT_CHOICE}"
fi

IMG="${_REPO}:${SNAPSHOT_CHOICE}" yq eval -i '.spec.image = env(IMG)' hack/catalog/catalogsource.yaml
oc create ns backplane-operator-system --dry-run=client -o yaml | oc apply -f -
oc apply -k hack/catalog/


_attempts=0
until oc apply -k config/samples >/dev/null 2>&1
do
  echo "INFO: Waiting for API to become available ..."
  _attempts=$((_attempts+1))
  if [ $_attempts -gt 10 ]; then
    echo "ERROR: cluster manager backplane subscription did not become available in time"
    exit 1
  fi
  sleep 10
done

echo "backplaneconfig installed succussfully"