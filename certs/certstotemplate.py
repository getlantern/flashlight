#!/usr/bin/env python

from os import walk
from jinja2 import Environment, FileSystemLoader
import inspect, os
import sys, getopt
from collections import OrderedDict

from filtercerts import iscert

def main(argv):
    script = inspect.getfile(inspect.currentframe())
    template = ''
    output = ''

    try:
        opts, args = getopt.getopt(argv,"ht:o:",["template=","output="])
    except getopt.GetoptError:
        usage(script)
        sys.exit(2)

    if (len(argv) < 4):
        usage(script)
        sys.exit(2)

    for opt, arg in opts:
        if opt == '-h':
            usage(script)
            sys.exit()
        elif opt in ("-t", "--template"):
            template = arg
        elif opt in ("-o", "--output"):
            output = arg

    generate_cloud(template, output, script)

def usage(script):
    print 'Usage: %s -t <templatefile> -o <outputfile>' % script

def load_cert(filename):
    with open (filename, "r") as myfile:
        data = myfile.read().replace('\n', '\\n')
    return data

def generate_cloud(template, output, script):
    certs = {}
    f = []
    for (dirpath, dirnames, filenames) in walk("."):
        f.extend(filenames)
        break
    for fn in filenames:
        if iscert(fn):
            certs[fn] = load_cert(fn)
    env = Environment(loader=FileSystemLoader("."))
    template = env.get_template(template)
    ordered = OrderedDict(sorted(certs.items(), key=lambda t: t[0]))
    rendered = template.render(masquerades=ordered)

    with open(output, "w") as cloudfile:
    	cloudfile.write(rendered)


if __name__ == "__main__":
    main(sys.argv[1:])



