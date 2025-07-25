#!/bin/bash -xe

apt-get update
apt-get install -y snapd
snap wait core seed.loaded
