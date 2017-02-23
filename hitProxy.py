#!/usr/bin/env python

import os
import sys
import json
import yaml
import tempfile
import subprocess

if len(sys.argv) != 2:
    print "Usage: %s <ip>" % sys.argv[0]
    sys.exit(1)

ip = sys.argv[1]

try:
    subprocess.check_call(["scp", "lantern@" + ip + ":access_data.json", "."])
except subprocess.CalledProcessError:
    print "Error copying access data from "+ip
    sys.exit(1)
except OSError:
    print "Error running scp"
    sys.exit(1)

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

subprocess.call(["./lantern", "-stickyconfig", "-readableconfig", "-configdir="+tmpdir])
