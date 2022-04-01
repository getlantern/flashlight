# For now the info name is generalized even though we only include the global
# config. Only the file path names are used by flashlight so this is
# arbitrary for now.
INFO_NAME =
TORRENT_CREATE ?= bin/torrent-create
DHT ?= bin/dht
TORRENT ?= bin/torrent
SEQ = 0
SALT = globalconfig
NAME = globalconfig
SHELL=/bin/sh -o pipefail

all: deps clean publish

publish: put-dht put-source

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
		s3://globalconfig.flashlightproxy.com/

$(NAME).target: dht-private-key
	$(DHT) derive-put-target mutable --private --key "$$(cat dht-private-key)" --salt "$(SALT)" > "$@"

$(NAME).torrent: $(NAME) $(NAME)/global.yaml.gz
	# We need this dir to only contain what we expect. I'm uncomfortable with
	# recursively blowing it away.
	rm -fv $(NAME)/.torrent.db
	$(TORRENT_CREATE) -i='$(CONFIG_INFO_NAME)' '-u=https://globalconfig.flashlightproxy.com/' $(NAME) > $@~
	mv $@~ $@

$(NAME):
	mkdir $@

$(NAME)/global.yaml.gz:
	curl -Ssf https://globalconfig.flashlightproxy.com/global.yaml.gz -o $@

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
	go install github.com/anacrolix/torrent/cmd/torrent@dd1ca6f51475529b432dba669bd84444f97043be

.PHONY: bin/torrent-create
bin/torrent-create:
	go install github.com/anacrolix/torrent/cmd/torrent-create@a319506dda5e63b4aa09dde762750689dfb1520b

get: $(NAME).target
	$(DHT) get `head -n 1 $(NAME).target` --salt $(SALT) --extract-infohash

deps: bin/dht bin/torrent bin/torrent-create

seed:
	@echo seeding $$(cat $(NAME).infohash)
	cd globalconfig && ../$(TORRENT) download --seed --no-progress ../$(NAME).torrent

clean:
	rm -rvf $(NAME)
	rm -f $(NAME).torrent $(NAME).infohash
