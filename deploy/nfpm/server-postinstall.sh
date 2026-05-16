#!/bin/sh
set -e
systemctl daemon-reload
systemctl enable kave-server.service || true
