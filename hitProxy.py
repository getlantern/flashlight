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

r = ru.redis_shell

on_windows = len([item for item in platform.uname() if "Microsoft" in item]) > 0

def create_tmpdir():
    if on_windows:
        wintemp = subprocess.check_output('wslpath $(cmd.exe /C "echo %TEMP%")', shell=True).strip()
        return tempfile.mkdtemp(dir=wintemp)
    else:
        return tempfile.mkdtemp(prefix="lantern")

def run_with_configdir(configdir, sticky=True, headless=False):
    path = ""
    if on_windows:
        path = "./lantern-cli.exe"
        # Make sure we compiled in console-mode as Gui mode won't take our command-line flags
        subprocess.call('file lantern.exe | grep console', shell=True)
        # Get Windows path for tempdir
        configdir = subprocess.check_output('wslpath -w {}'.format(tmpdir), shell=True).strip()
    elif os.path.isfile("./lantern"):
        path = "./lantern"
    else:
        path = "/Applications/Lantern.app/Contents/MacOS/lantern"

    args = [path, "-pprofaddr=:4000", "-readableconfig", "-configdir="+configdir]
    if sticky:
        args = args + ["-stickyconfig"]
    if headless:
        args = args + ["-headless"]
    subprocess.call(args)


def config_for(name_or_ip, remote_user="lantern"):
    if ipre.match(name_or_ip):
        name = r.hget('srvip->server', name_or_ip)
    else:
        name = name_or_ip

    access_data = r.hget('server->config', name) if name else None
    if access_data:
        print "Loaded access data from redis"
        return yaml.load(access_data).values()
    else:
        print "No access data found in redis, fetching directly from server"

        ip = name_or_ip
        # Allow caller to also input server names
        if not ipre.match(ip):
            ip = r.hget('server->srvip', name_or_ip)
        if not ip:
            print "Unable to resolve %s to ip" % (name_or_ip)
            sys.exit(1)
        try:
            subprocess.check_call(["scp", "%s@%s:access_data.json" % (remote_user, ip), "."])
        except subprocess.CalledProcessError:
            print "Error copying access data from "+ip
            sys.exit(1)
        except OSError:
            print "Error running scp"
            sys.exit(1)

        with open("access_data.json") as ad:
            return yaml.load(ad.read())

if __name__ == '__main__':
    if len(sys.argv) < 2:
        print "Usage: %s <name or ip> [user]" % sys.argv[0]
        sys.exit(1)

    name_or_ip = sys.argv[1]
    remote_user = "lantern"
    if len(sys.argv) > 2:
        remote_user = sys.argv[2]
    cfgs = config_for(name_or_ip, remote_user)
    print repr(cfgs)

    servers = {}
    for i, cfg in enumerate(cfgs):
        cfg['trusted'] = True
        servers['server-%s' % i] = cfg


    configdir = create_tmpdir()
    try:
        with open(os.path.join(configdir, "proxies.yaml"), "w") as f:
            cfg = yaml.safe_dump(servers, encoding='utf-8', allow_unicode=True, default_flow_style=False)
            f.write(cfg)
        print cfg
        run_with_configdir(configdir)
    finally:
        shutil.rmtree(configdir)
