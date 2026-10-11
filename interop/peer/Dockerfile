# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# tinytuya (MIT, github.com/jasonacox/tinytuya) as an interop peer; it is
# installed from PyPI at build time, not redistributed.
FROM python:3.14-slim
RUN pip install --no-cache-dir tinytuya==1.20.0
COPY client.py /peer/client.py
WORKDIR /peer
