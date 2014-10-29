#!/usr/bin/env python

import inspect
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

import certstotemplate


go_config_header = """\
package config

import "github.com/getlantern/flashlight/client"

var cloudflareMasquerades = []*client.Masquerade{
"""


def addmasquerades(domains, refreshcerts=False):
	if not in_same_directory():
		# I tried making this robust to being called from elsewhere, but that
		# wouldn't work with other scripts that this uses, and as long as the
		# user gets a clear error message this is not really a problem.
		print "You must call this from the same directory (%s/)" % here()
		sys.exit(1)
	any_errors = False
	with file('errors.txt', 'w') as errors:
		for domain in domains:
			if refreshcerts or not os.path.exists(domain):
				print "Getting cert for %s ..." % domain
				# I call get_rootca before I create the file so an empty file
				# won't be left around if get_rootca fails.
				try:
					rootca = get_rootca(domain)
					file(domain, 'w').write(rootca)
				except KeyboardInterrupt:
					raise
				except ErrorGettingCert:
					any_errors = True
					traceback.print_exc()
					errors.write(domain + '\n')
	if any_errors:
		print "Some errors were detected while fetching certs."
		print "The offending domains have been collected in the errors.txt file"
		print "So you can retry those by running %s errors.txt" % thisfilename()
		print "Do you want to go on to update cloud.yaml and masquerades.go"
		print "anyway? (y/N)"
		if raw_input().strip().lower() != 'y':
			sys.exit(1)
	certstotemplate.main(['-t', 'cloud.yaml.tmpl',
		                  '-o', 'cloud.yaml'])
	cloud_config = yaml.load(file('cloud.yaml'))
	fronts = cloud_config['client']['masqueradesets']['cloudflare']
	# Sort to make git diffs more helpful.
	fronts.sort(key=lambda d: d['domain'])
	with file(os.path.join('..', 'config', 'masquerades.go'), 'w') as out:
		out.write(go_config_header)
		for f in fronts:
			out.write('\t&client.Masquerade{\n')
			out.write('\t\tDomain: "%s",\n' % f['domain'])
			out.write('\t\tRootCA: "%s",\n' % f['rootca'].replace('\n', '\\n'))
			out.write('\t},\n')
		out.write('}\n')
	if (raw_input("Do you want to upload cloud.yaml? (y/N)").strip().lower()
		== "y"):
		print "OK, trying..."
		# XXX: boto?
		result = os.system("./updateyaml.bash")
		if result != 0:
			print "Do you have s3cmd installed and configured?"

class ErrorGettingCert(Exception):
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
		except ssl.Error:
			pass
	raise ErrorGettingCert

def thisfilename():
	return inspect.getfile(inspect.currentframe())

def here():
	return os.path.dirname(thisfilename())

def in_same_directory():
	return os.path.abspath(here()) == os.getcwd()


if __name__ == '__main__':
	if len(sys.argv) != 2:
		print "Usage: %s <masquerades file>" % sys.argv[0]
		print "Where <masquerades file> should be the path of a file"
		print "containing domain names separated by whitespace."
		sys.exit(1)
	addmasquerades(filter(None,
						  map(str.strip,
							  file(sys.argv[1]).read().split())))


