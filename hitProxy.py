#!/usr/bin/env python

import os
import sys
import yaml
import tempfile
import subprocess
import os.path
from misc_util import ipre
import redis_util as ru

r = ru.redis_shell

if len(sys.argv) < 2:
    print "Usage: %s <name or ip> [user]" % sys.argv[0]
    sys.exit(1)

name_or_ip = sys.argv[1]
if ipre.match(name_or_ip):
    name = r.hget('srvip->server', name_or_ip)
else:
    name = name_or_ip

access_data = r.hget('server->config', name)
if access_data:
    print "Loaded access data from redis"
    cfgs = yaml.load(access_data).values()
else:
    print "No access data found in redis, fetching directly from server"

    ip = name_or_ip
    # Allow caller to also input server names
    if not ipre.match(ip):
        ip = r.hget('server->srvip', name_or_ip)

    if not ip:
        print "Unable to resolve %s to ip" % (name_or_ip)

    user = "lantern"
    if len(sys.argv) > 2:
        user = sys.argv[2]
    try:
        subprocess.check_call(["scp", "%s@%s:access_data.json" % (user, ip), "."])
    except subprocess.CalledProcessError:
        print "Error copying access data from "+ip
        sys.exit(1)
    except OSError:
        print "Error running scp"
        sys.exit(1)

    with open("access_data.json") as ad:
        cfgs = yaml.load(ad.read())

print repr(cfgs)

servers = {}
for i, cfg in enumerate(cfgs):
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

subprocess.call([path, "-headless", "-pprofaddr=:4000", "-stickyconfig", "-readableconfig", "-configdir="+tmpdir])
