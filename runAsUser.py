#!/usr/bin/env python

import os
import os.path
from os import path
import platform
import shutil
import subprocess
import sys
import tempfile
import yaml
from misc_util import ipre
import redis_util as ru
import hitProxy
import json

r = ru.redis_shell

if len(sys.argv) < 2:
    print "Usage: %s user-id [args]" % sys.argv[0]
    sys.exit(1)

user_id = sys.argv[1]

# Since we'll often be running via a VPN with this script, use a little local cache to avoid
# redis calls that may fail.
user_file = '.rediscache.json'
token = None
if not path.isfile(user_file):
    data = {}
else:
    with open(user_file) as json_file:
        data = json.load(json_file)
        if user_id in data:
            token = data[user_id]

if not token:
    token = r.hget('user->token', user_id)

if not token:
    print "Could not get user token. Is REDIS_URL set correctly as '{}'?".format(os.getenv("REDIS_URL"))
    sys.exit(1)

data[user_id] = token
with open(user_file, 'w') as outfile:
    json.dump(data, outfile)

configdir = hitProxy.create_tmpdir()

try:
    with open(os.path.join(configdir, "settings.yaml"), "w") as f:
        cfg = yaml.safe_dump({
            'userID': int(user_id),
            'userToken': token,
            'proxyAll': 'true',
        }, encoding='utf-8', allow_unicode=True, default_flow_style=False)
        f.write(cfg)
    args = ["-headless"]
    args.extend(sys.argv[2:])
    hitProxy.run_with_configdir(configdir, args)
finally:
    shutil.rmtree(configdir)
