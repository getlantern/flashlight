#!/usr/bin/env python

from os import walk
from jinja2 import Environment, FileSystemLoader
import inspect, os

def load_cert(filename):
    with open (filename, "r") as myfile:
        data = myfile.read().replace('\n', '\\n')
    return data

def generate_cloud():
    certs = {}
    f = []
    for (dirpath, dirnames, filenames) in walk("."):
        f.extend(filenames)
        break

    script = inspect.getfile(inspect.currentframe())
    for fn in filenames:
    	if ("yaml" not in fn) and (fn not in script):
    		certs[fn] = load_cert(fn)


    env = Environment(loader=FileSystemLoader("."))
    template = env.get_template('cloud.yaml.tmpl')
    rendered = template.render(masquerades=certs)

    with open("cloud.yaml", "w") as cloudfile:
    	cloudfile.write(rendered)


generate_cloud()
