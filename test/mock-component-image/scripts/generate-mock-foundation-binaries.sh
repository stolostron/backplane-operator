#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project

if [ -d "./bin" ]; then
    rm -rf ./bin
fi

mkdir ./bin

GOOS=linux go build ./templates/controller/ocm-controller.go
mv ocm-controller ./bin/controller

GOOS=linux go build ./templates/webhook/ocm-webhook.go
mv ocm-webhook ./bin/webhook

GOOS=linux go build ./templates/proxyserver/ocm-proxyserver.go
mv ocm-proxyserver ./bin/proxyserver

curl -o bin/kubectl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x bin/kubectl