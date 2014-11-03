#!/usr/bin/env python

from __future__ import division

import inspect
from collections import OrderedDict
import os
import socket
import sys
import traceback

# sudo pip install pyopenssl
# If you get an AttributeError: get_peer_cert_chain, try the --upgrade flag
# above.
from OpenSSL import SSL as ssl, crypto

try:
    ssl.TLSv1_2_METHOD
except AttributeError:
    print "Bad OpenSSL version.  Try `pip install --upgrade pyopenssl` ?"
    sys.exit(1)

# sudo pip install pyyaml
import yaml


go_config_header = """\
package config

import "github.com/getlantern/flashlight/client"

var cloudflareMasquerades = []*client.Masquerade{
"""


def addmasquerades(domains):
    def update(masquerades):
        new_domains = domains - existing_domains(masquerades)
        if not new_domains:
            print "No new domains!"
            sys.exit(1)
        return extend_masquerades(masquerades[:], new_domains)
    return apply_update(update)

def refresh(domains):
    def update(masquerades):
        if domains == 'ALL':
            d = existing_domains(masquerades)
        else:
            d = domains
        return extend_masquerades([m for m in masquerades
                                   if m['domain'] not in d],
                                  d)
    return apply_update(update)

def extend_masquerades(out, domains):
    errors = []
    total = len(domains)
    for i, d in enumerate(sorted(domains)):
        print "Fetching data for %-30s %9s/%s (%.2f%%)..." % (
                d, i, total, i*100/total)
        try:
            out.append(get_masquerade(d))
        except ErrorFetchingData:
            errors.append(d)
    return out, errors

def apply_update(update_fn):
    cfg_path = rel2here('cloud.yaml')
    cfg = yaml.load(file(cfg_path))
    masquerades, errors = update_fn(cfg['client']['masqueradesets']['cloudflare'])
    if errors:
        file(rel2here('errors.txt'), 'w').write('\n'.join(errors))
        print "Some errors were detected while fetching masquerade data."
        print "The offending domains have been collected in the errors.txt file"
        print "So you can retry those by running %s errors.txt" % thisfilename()
        print "Do you still want to update cloud.yaml and masquerades.go? (y/N)"
        if raw_input().strip().lower() != 'y':
            sys.exit(1)
    cfg['client']['masqueradesets']['cloudflare'] = masquerades
    pretty_dump(cfg, file(cfg_path, 'w'))
    write_go(masquerades)
    if (raw_input("Do you want to upload cloud.yaml? (y/N)").strip().lower()
        == "y"):
        print "OK, trying..."
        result = os.system(rel2here("updateyaml.bash"))
        if result != 0:
            print ("Do you have s3cmd installed and configured? (see %s)"
                   % rel2here('..', 'README.md'))

def write_go(masquerades):
    with file(rel2here('..', 'config', 'masquerades.go'), 'w') as out:
        out.write(go_config_header)
        for m in masquerades:
            out.write('\t&client.Masquerade{\n')
            out.write('\t\tDomain: "%s",\n' % m['domain'])
            out.write('\t\tIPAddress: "%s",\n' % m.get('ipaddress', ''))
            out.write('\t\tRootCA: "%s",\n' % m['rootca'].replace('\n', '\\n'))
            out.write('\t},\n')
        out.write('}\n')

def get_masquerade(domain):
    try:
        ip = socket.gethostbyname(domain)
    except IOError:
        traceback.print_exc()
        raise ErrorFetchingData
    return {'domain': domain,
            'ipaddress': ip,
            'rootca': get_rootca(domain)}

class ErrorFetchingData(Exception):
    pass

def get_rootca(domain):
    for version in [ssl.TLSv1_2_METHOD,
                    ssl.TLSv1_1_METHOD,
                    ssl.TLSv1_METHOD]:
        try:
            ctx = ssl.Context(version)
            ctx.set_verify(ssl.VERIFY_NONE, lambda *args: True)
            s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            try:
                conn = ssl.Connection(ctx, s)
                conn.connect((domain, 443))
                conn.do_handshake()
                chain = conn.get_peer_cert_chain()
                # rootca is last in the chain
                return crypto.dump_certificate(crypto.FILETYPE_PEM, chain[-1])
            finally:
                s.close()
        except (IOError, ssl.Error):
                        traceback.print_exc()
    raise ErrorFetchingData

def existing_domains(masquerades):
    return set(m['domain'] for m in masquerades)

def thisfilename():
    return inspect.getfile(inspect.currentframe())

def here():
    return os.path.dirname(thisfilename())

def rel2here(*path):
    return os.path.join(here(), *path)

def read_words(path):
    return set(filter(None, map(str.strip, file(path).read().split())))

class certstr(str):
    pass

def die():
    cmd = sys.argv[0]
    print
    print "Usage:"
    print "    %s add <masquerades file>" % cmd
    print "    %s refresh [<masquerades file>]" % cmd
    print
    print "Where <masquerades file> should be the path of a file containing"
    print "domain names separated by whitespace."
    print
    print "If you call `refresh` with no <masquerades file>, all domains"
    print "will be refreshed."
    print
    sys.exit(1)

# Prettify YAML

def pretty_dump(cfg, stream):
    od = OrderedDict()
    cfg['client']['servers'] = map(sorted_server, cfg['client']['servers'])
    cfg['client'] = sorted_dict(cfg['client'], 'servers', 'masqueradesets')
    masquerades = cfg['client']['masqueradesets']['cloudflare']
    masquerades.sort(key=lambda d: d['domain'])
    for d in masquerades:
        d['rootca'] = certstr(d['rootca'])
    yaml.dump(cfg, stream, default_flow_style=False)

def sorted_server(d):
    return sorted_dict(d,
                      'host',
                      'port',
                      'masqueradeset',
                      'insecureskipverify',
                      'dialtimeoutmillis',
                      'redialattempts',
                      'keepalivemillis',
                      'weight',
                      'qos')

def sorted_dict(d, *keys):
    # If the keys don't exactly match what we expect don't crash, since this is
    # cosmetic stuff anyway, but make sure we don't lose any.
    d = d.copy()
    od = OrderedDict()
    for k in keys:
        try:
            od[k] = d.pop(k)
        except KeyError:
            print "WARNING: key %s not found" % k
    for k, v in sorted(d.iteritems()):
        print "WARNING: unexpected key %s found" % k
        od[k] = v
    return od

def odict_representer(dumper, data):
    return dumper.represent_mapping(
            yaml.resolver.BaseResolver.DEFAULT_MAPPING_TAG,
            data.items())
yaml.add_representer(OrderedDict, odict_representer)

def cert_representer(dumper, data):
    return dumper.represent_scalar('tag:yaml.org,2002:str', data, style='|')
yaml.add_representer(certstr, cert_representer)


if __name__ == '__main__':
    if len(sys.argv) < 2:
        die()
    if sys.argv[1] == 'add':
        if len(sys.argv) == 3:
            addmasquerades(read_words(sys.argv[2]))
        else:
            die()
    elif sys.argv[1] == 'refresh':
        if len(sys.argv) == 2:
            refresh('ALL')
        elif len(sys.argv) == 3:
            refresh(read_words(sys.argv[2]))
        else:
            die()
    else:
        die()


