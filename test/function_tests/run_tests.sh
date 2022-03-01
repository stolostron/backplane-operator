#!/bin/bash
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project



echo "Starting Backplane Functional Tests ..."
echo ""

if [ -z "$TEST_MODE" ]; then
    echo "TEST_MODE not exported. Must be of type 'install'"
    exit 1
fi



if [[ "$TEST_MODE" == "install" ]]; then
    echo "Beginning Backplane Tests ..."
    echo ""
    ginkgo -tags functional -v --slow-spec-threshold=300s test/function_tests/backplane_operator_install_test
fi