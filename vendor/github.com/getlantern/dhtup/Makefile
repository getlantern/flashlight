TORRENT_CREATE ?= bin/torrent-create
DHT ?= bin/dht
TORRENT ?= bin/torrent
SEQ = 0
# This is the default. We should remove it and enforce that it is set. Currently `replica` and
# `globalconfig` are supported.
NAME = globalconfig
SALT = $(NAME)
BUCKET = globalconfig.flashlightproxy.com
# The trailing / is important for both aws cli and BitTorrent web-seeding.
S3_UPLOAD_ROOT = $(BUCKET)/dhtup/
WEBSEED_URL = https://$(S3_UPLOAD_ROOT)
SHELL=/bin/sh -o pipefail

all: deps clean publish

publish: put-dht put-source put-webdata

put-dht: $(NAME).infohash dht-private-key
	# TODO: Update the salt to "config" once we aren't hardcoding the target infohash.
	$(DHT) put-mutable-infohash \
		--key `cat dht-private-key` \
		--salt $(SALT) \
		--info-hash "`cat $(NAME).infohash`" \
		--seq '$(SEQ)' \
		--auto-seq \
	| tee $(NAME).target

put-source: $(NAME).torrent
	aws s3 cp --acl public-read \
		$(NAME).torrent \
		s3://$(S3_UPLOAD_ROOT)

put-webdata: $(NAME)
	# aws cli uses Docker's dumbass directory handling
	aws s3 cp --recursive --acl public-read \
		$(NAME) \
		s3://$(S3_UPLOAD_ROOT)$(NAME)

$(NAME).target: dht-private-key
	$(DHT) derive-put-target mutable --private --key "$$(cat dht-private-key)" --salt "$(SALT)" > "$@"

.PHONY: torrent
torrent: $(NAME).torrent

# Couldn't find a better way to embed the dependencies of $(NAME).torrent dynamically. They're
# stuffed into a variable and unpacked with SECONDEXPANSION.
globalconfig.files = globalconfig/global.yaml.gz
replica.files = replica/backup-search-index.db

.SECONDEXPANSION:
$(NAME).torrent: $(NAME) $$($$(NAME).files)
	# We need this dir to only contain what we expect. I'm uncomfortable with
	# recursively blowing it away.
	rm -fv $(NAME)/.torrent.db
	# The trackers are TCP with IPv6 addresses. See
	# https://github.com/getlantern/lantern-internal/issues/5469. Use the tracker list from
	# trackers.go.
	$(TORRENT_CREATE) \
		'-u=$(WEBSEED_URL)' \
		'-n' \
		'-a=udp://opentor.org:2710/announce' \
		'-a=http://tracker4.itzmx.com:2710/announce' \
		'-a=udp://tracker.opentrackr.org:1337/announce' \
		'-a=https://tracker.nanoha.org:443/announce' \
		'-a=http://t.nyaatracker.com:80/announce' \
		$(NAME) > $@~
	mv $@~ $@

$(NAME):
	mkdir -p $(NAME)


globalconfig/global.yaml.gz:
	curl -Ssf https://globalconfig.flashlightproxy.com/global.yaml.gz -o $@

replica/backup-search-index.db:
	curl -Ssf https://replica-rust-frankfurt-staging.herokuapp.com/backup-search-index -o $@

$(NAME).infohash: $(NAME).torrent
	$(TORRENT) metainfo $< infohash | cut -d : -f 1 > $@

dht-private-key:
	openssl rand -hex 32 > $@

export GOBIN=$(shell echo `pwd`/bin)

.PHONY: bin/dht
bin/dht:
	go install github.com/anacrolix/dht/v2/cmd/dht@114cb152af7c452f70f90f3e81c41495a855a70e

.PHONY: bin/torrent
bin/torrent:
	go install github.com/anacrolix/torrent/cmd/torrent@1f6b23d995114355fa3081dcda5422ea8fa6766f

.PHONY: bin/torrent-create
bin/torrent-create:
	go install github.com/anacrolix/torrent/cmd/torrent-create@a319506dda5e63b4aa09dde762750689dfb1520b

get: $(NAME).target
	$(DHT) get `head -n 1 $(NAME).target` --salt $(SALT) --extract-infohash

deps: bin/dht bin/torrent bin/torrent-create

# Don't use this from inside Fly. It's just a one-off for seeding a specific resource interactively.
seed: $(NAME).torrent $(NAME).infohash
	@echo seeding $$(cat $(NAME).infohash)
	$(TORRENT) download --seed --no-progress $(NAME).torrent

clean:
	rm -rvf $(NAME)
	rm -fv $(NAME).torrent $(NAME).infohash
