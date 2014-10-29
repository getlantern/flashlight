#!/usr/bin/env python

import os
import sys

from addmasquerades import addmasquerades
from filtercerts import iscert


def refreshcerts(filename=None):
	if filename is None:
		domains = [name for name in os.listdir('.') if iscert(name)]
	else:
		domains = map(str.strip, file(filename).read().split())
	addmasquerades(domains,
				   refreshcerts=True)


if __name__ == '__main__':
	if len(sys.argv) == 1:
		refreshcerts(None)
	elif len(sys.argv) == 2:
		refreshcerts(sys.argv[1])
	else:
		print "Usage: %s [<filename>]" % sys.argv[0]
		print "If provided, <filename> should be the path of a file containing"
		print "a whitespace-separated sequence of domain names."
		print "If omitted, all certs will be refreshed."
