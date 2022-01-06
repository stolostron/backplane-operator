#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

# Clones hub-crds repo and copies foundation crds into the bin/crds directory

crds_dir="pkg/templates/crds/foundation" # where to place crds
branch=$(cat hack/scripts/targetCRDs.txt) # what branch to checkout
crds_path="crds/foundation" # where to copy crds from

# Remove existing files
rm ${crds_dir}/*.yaml
rm -rf crd-temp
mkdir -p crd-temp
mkdir -p ${crds_dir}

# Clone hub-crds into crd-temp
git clone --depth=1 --branch ${branch} https://github.com/stolostron/hub-crds  crd-temp

# Update sha
sha=$(cd crd-temp && git rev-parse HEAD)
echo -n $sha > hack/scripts/currentCRDs.txt

# Copy foundation yaml files
find crd-temp/${crds_path} -name \*.yaml -exec cp {} ${crds_dir}  \;

# Delete clone directory
rm -rf crd-temp
