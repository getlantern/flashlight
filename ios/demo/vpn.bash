#!/bin/bash

source ./routes.bash

sudo route delete default
sudo route add $PROXY $GATEWAY
sudo route add $REAL_DNS $GATEWAY
sudo route add default $TUN_GATEWAY