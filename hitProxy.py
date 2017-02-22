#!/usr/bin/env python

import os
import sys
import json
import yaml
import tempfile
from subprocess import call

ip = sys.argv[1]
call(["scp", "lantern@" + ip + ":access_data.json", "."])
loaded = json.loads(open("access_data.json").read())

servers = {}

for i, server in enumerate(loaded):
    server["trusted"] = True
    servers["server-"+str(i)] = server

tmpdir = tempfile.mkdtemp()
p = os.path.join(tmpdir, "proxies.yaml")
f = open(p, "w")
f.write(yaml.safe_dump(servers, encoding='utf-8', allow_unicode=True, default_flow_style=False))
f.close()

call(["./lantern", "-stickyconfig", "-readableconfig", "-configdir="+tmpdir])
