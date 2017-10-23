#!/usr/bin/env python

import os
import sys
import json
import yaml
import tempfile
import subprocess
import os.path
from misc_util import ipre
import redis_util as ru

r = ru.redis_shell

if len(sys.argv) != 2:
    print "Usage: %s <ip>" % sys.argv[0]
    sys.exit(1)

ip = sys.argv[1]

# Allow caller to also input server names
if not ipre.match(ip):
    ip = r.hget('server->srvip', ip)

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
cfg = yaml.safe_dump(servers, encoding='utf-8', allow_unicode=True, default_flow_style=False)
f.write(cfg)
f.close()
print cfg

path = ""
if os.path.isfile("./lantern"):
    path = "./lantern"
else:
    path = "/Applications/Lantern.app/Contents/MacOS/lantern"

subprocess.call([path, "-stickyconfig", "-readableconfig", "-configdir="+tmpdir])
