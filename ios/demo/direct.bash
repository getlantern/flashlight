#!/bin/bash

source ./routes.bash

sudo route delete default
sudo route delete $PROXY
sudo route delete $REAL_DNS
sudo route add default $GATEWAY