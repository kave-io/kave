#!/bin/sh
set -e
systemctl stop kave-server.service || true
systemctl disable kave-server.service || true
