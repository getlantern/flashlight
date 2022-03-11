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

all: deps publish

publish: $(NAME).infohash dht-private-key
	# TODO: Update the salt to "config" once we aren't hardcoding the target infohash.
	$(DHT) put-mutable-infohash \
		--key `cat dht-private-key` \
		--salt $(SALT) \
		--info-hash "`cat $(NAME).infohash`" \
		--seq '$(SEQ)' \
		--auto-seq \
	| tee $(NAME).target

$(NAME).torrent: $(NAME) $(NAME)/global.yaml.gz
	$(TORRENT_CREATE) -i='$(CONFIG_INFO_NAME)' '-u=https://globalconfig.flashlightproxy.com/' $(NAME) > $@~
	mv $@~ $@

$(NAME):
	mkdir $@

$(NAME)/global.yaml.gz:
	curl https://globalconfig.flashlightproxy.com/global.yaml.gz -o $@

$(NAME).infohash: $(NAME).torrent
	$(TORRENT) metainfo $< infohash | cut -d : -f 1 > $@

dht-private-key:
	openssl rand -hex 32 > $@

.PHONY: bin/dht
bin/dht:
	GOBIN=`realpath bin` go install github.com/anacrolix/dht/v2/cmd/dht@5fb252416efe1c24656b60a835cf680edbd67766

.PHONY: bin/torrent
bin/torrent:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent@latest

.PHONY: bin/torrent-create
bin/torrent-create:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent-create@latest

get:
	$(DHT) get `head -n 1 $(NAME).target` --salt $(SALT) --extract-infohash

deps: bin/dht bin/torrent bin/torrent-create
