# For now the info name is generalized even though we only include the global
# config. Only the file path names are used by flashlight so this is
# arbitrary for now.
CONFIG_INFO_NAME = config
TORRENT_CREATE = bin/torrent-create
DHT = bin/dht
TORRENT = bin/torrent
SEQ = 0
SALT = globalconfig

all: deps publish

publish: $(CONFIG_INFO_NAME).infohash dht-private-key
	# TODO: Update the salt to "config" once we aren't hardcoding the target infohash.
	$(DHT) put-mutable-infohash \
		--key `cat dht-private-key` \
		--salt $(SALT) \
		--info-hash "`cat $(CONFIG_INFO_NAME).infohash`" \
		--seq '$(SEQ)' \
	| tee $(CONFIG_INFO_NAME).target


$(CONFIG_INFO_NAME).torrent: $(CONFIG_INFO_NAME) $(CONFIG_INFO_NAME)/global.yaml
	$(TORRENT_CREATE) $(CONFIG_INFO_NAME) > $@

$(CONFIG_INFO_NAME):
	mkdir $@

$(CONFIG_INFO_NAME)/global.yaml:
	curl https://globalconfig.flashlightproxy.com/global.yaml.gz | gunzip > $@

# $(CONFIG_INFO_NAME)/proxies.yaml:
# 	curl https://config.getiantem.org/proxies.yaml.gz | gunzip > $@

$(CONFIG_INFO_NAME).infohash: $(CONFIG_INFO_NAME).torrent
	$(TORRENT) metainfo $< infohash | cut -d : -f 1 > $@

dht-private-key:
	openssl rand -hex 32 > $@

.PHONY: bin/dht
bin/dht:
	GOBIN=`realpath bin` go install github.com/anacrolix/dht/v2/cmd/dht@3791ab26c002c8c358b29130693d7812cb420cca

.PHONY: bin/torrent
bin/torrent:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent@latest

.PHONY: bin/torrent-create
bin/torrent-create:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent-create@latest

get:
	$(DHT) get `head -n 1 $(CONFIG_INFO_NAME).target` --salt $(SALT) --extract-infohash

deps: bin/dht bin/torrent bin/torrent-create
