#!/usr/bin/env python

import os
import os.path
import platform
import shutil
import subprocess
import sys
import tempfile
import yaml
from misc_util import ipre
import redis_util as ru
import hitProxy

r = ru.redis_shell

if len(sys.argv) < 2:
    print "Usage: %s user-id [country]" % sys.argv[0]
    sys.exit(1)

user_id = sys.argv[1]
country = ""
if len(sys.argv) > 2:
    country = sys.argv[2]

token = r.hget('user->token', user_id)

if not token:
    print "Could not get user token. Is REDIS_URL set correctly as '{}'?".format(os.getenv("REDIS_URL"))
    sys.exit(1)

configdir = hitProxy.create_tmpdir()

try:
    with open(os.path.join(configdir, "settings.yaml"), "w") as f:
        cfg = yaml.safe_dump({
            'userID': int(user_id),
            'userToken': token,
            'proxyAll': 'true',
        }, encoding='utf-8', allow_unicode=True, default_flow_style=False)
        f.write(cfg)
    hitProxy.run_with_configdir(configdir, sticky=False, headless=True, country=country)
finally:
    shutil.rmtree(configdir)
