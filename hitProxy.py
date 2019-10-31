#!/usr/bin/env python

import argparse
import os
import os.path
import platform
import shutil
import subprocess
import sys
import tempfile
import yaml

from misc_util import ipre

on_windows = len([item for item in platform.uname() if "Microsoft" in item]) > 0

def create_tmpdir():
    if on_windows:
        wintemp = subprocess.check_output('wslpath $(cmd.exe /C "echo %TEMP%")', shell=True).strip()
        return tempfile.mkdtemp(dir=wintemp)
    else:
        return tempfile.mkdtemp(prefix="lantern")

def run_with_configdir(configdir, sticky=True, headless=False, country="", remainder=[]):
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
    if country:
        args = args + ["-force-config-country="+country]
    subprocess.call(args + remainder)


def config_for(name_or_ip, direct, remote_user='lantern'):
    ip = name_or_ip # assuming IP in case when direct is true

    if not direct:
        from redis_util import redis_shell as r
        if ipre.match(name_or_ip):
            name = r.hget('srvip->server', name_or_ip)
        else:
            name = name_or_ip
            ip = r.hget('server->srvip', name_or_ip)

        access_data = r.hget('server->config', name) if name else None
        if access_data:
            print "Loaded access data from redis"
            return yaml.load(access_data).values()
        else:
            print "No access data found in redis, fetching directly from server"

    if not ipre.match(ip):
        sys.exit("'%s' is not a valid IP address" % ip)
    try:
        subprocess.check_call(["scp", "%s@%s:access_data.json" % (remote_user, ip), "."])
    except subprocess.CalledProcessError:
        sys.exit("Error copying access data from "+ip)
    except OSError:
        print
        sys.exit("Error running scp")

    with open("access_data.json") as ad:
        return yaml.load(ad.read())

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Fetch config for one particular proxy and instructs Lantern to use it')
    parser.add_argument('name_or_ips', nargs='+', type=str, help='The name or ip of the proxy to hit')
    parser.add_argument('-u', '--remote-user', dest='remote_user', type=str, default='lantern', help='The SSH user on the proxy when fetching config from it')
    parser.add_argument('-d', '--direct', action='store_true', help='Skip Redis lookup and fetch directly from the proxy')
    parser.add_argument('remainder', nargs=argparse.REMAINDER, help='the rest of the arguments are passed directly to the Lantern binary')
    args = parser.parse_args()
    servers = {}
    for name_or_ip in args.name_or_ips:
        for i, cfg in enumerate(config_for(name_or_ip, args.direct, args.remote_user)):
            cfg['trusted'] = True
            servers['%s-%s' % (name_or_ip, i)] = cfg
    print repr(servers)

    configdir = create_tmpdir()
    try:
        with open(os.path.join(configdir, "proxies.yaml"), "w") as f:
            cfg = yaml.safe_dump(servers, encoding='utf-8', allow_unicode=True, default_flow_style=False)
            f.write(cfg)
        print cfg
        run_with_configdir(configdir, remainder=args.remainder)
    finally:
        shutil.rmtree(configdir)
