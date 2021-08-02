#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

set -e

_IMAGE_NAME="backplane-operator-bundle"
_WEB_REPO="https://quay.io/repository/open-cluster-management/${_IMAGE_NAME}?tab=tags"
_REPO="quay.io/open-cluster-management/${_IMAGE_NAME}"

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

IMG="${_REPO}:${SNAPSHOT_CHOICE}" yq eval -i '.spec.image = env(IMG)' hack/upstream-install/catalogsource.yaml