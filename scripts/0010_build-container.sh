#!/bin/bash

# uploader - A tool to upload files to various cloud storage providers
# Copyright (C) 2026 0x1115 Inc.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Capture version from command line argument
# Default to "latest" if no version is provided
VERSION=${1:-latest}

CONTAINER_REGISTRY="0x1115"
IMAGE_NAME="uploader"
IMAGE_TAG="${VERSION}"

# Build and push the Docker image for uploader

docker build -t ${CONTAINER_REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG} \
 -t ${CONTAINER_REGISTRY}/${IMAGE_NAME}:latest \
 --platform linux/amd64,linux/arm64 \
 -f build/Dockerfile .