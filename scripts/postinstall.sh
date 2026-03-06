#!/bin/sh
set -e
systemctl daemon-reload
systemctl enable netsense
