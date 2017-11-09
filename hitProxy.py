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
    print "Usage: %s <name or ip>" % sys.argv[0]
    sys.exit(1)

name_or_ip = sys.argv[1]
if ipre.match(name_or_ip):
    name = r.hget('srvip->server', name_or_ip)
else:
    name = name_or_ip

cfgs = yaml.load(r.hget('server->config', name))

print repr(cfgs)

servers = {}
for i, cfg in enumerate(cfgs.values()):
    cfg['trusted'] = True
    servers['server-%s' % i] = cfg

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

subprocess.call([path, "-headless", "-stickyconfig", "-readableconfig", "-configdir="+tmpdir])
