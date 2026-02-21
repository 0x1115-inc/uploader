#!/bin/bash

# Copyright 2025 0x1115 Inc <info@0x1115.com>
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Capture version from command line argument
# Default to "latest" if no version is provided
VERSION=${1:-latest}

CONTAINER_REGISTRY="0x1115"
IMAGE_NAME="uploader"
IMAGE_TAG="${VERSION}"

# Build and push the Docker image for uploader

docker build -t ${CONTAINER_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} \
 -t ${CONTAINER_REGISTRY}/${IMAGE_NAME}:latest \
 --platform linux/amd64,linux/arm64 .