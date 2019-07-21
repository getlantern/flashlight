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
    print "Usage: %s user-id" % sys.argv[0]
    sys.exit(1)

user_id = sys.argv[1]
token = r.hget('user->token', user_id)

configdir = hitProxy.create_tmpdir()
try:
    with open(os.path.join(configdir, "settings.yaml"), "w") as f:
        cfg = yaml.safe_dump({
            'userID': user_id,
            'userToken': token,
            'proxyAll': 'true',
        }, encoding='utf-8', allow_unicode=True, default_flow_style=False)
        f.write(cfg)
        hitProxy.run_with_configdir(configdir)
finally:
    shutil.rmtree(configdir)
