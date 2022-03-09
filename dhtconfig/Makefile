CONFIG_INFO_NAME = config
TORRENT_CREATE = bin/torrent-create
DHT = bin/dht
TORRENT = bin/torrent
SEQ = 0

publish: $(CONFIG_INFO_NAME).infohash bin/dht dht-private-key
	# TODO: Update the salt to "config" once we aren't hardcoding the target infohash.
	$(DHT) put-mutable-infohash \
		--key `cat dht-private-key` \
		--salt globalconfig \
		--info-hash "`cat $(CONFIG_INFO_NAME).infohash`" \
		--seq '$(SEQ)' \
	| tee $(CONFIG_INFO_NAME).target

$(CONFIG_INFO_NAME).torrent: $(CONFIG_INFO_NAME) $(CONFIG_INFO_NAME)/global.yaml $(CONFIG_INFO_NAME)/proxies.yaml $(TORRENT_CREATE)
	$(TORRENT_CREATE) root > $@

$(CONFIG_INFO_NAME):
	mkdir $@

$(CONFIG_INFO_NAME)/global.yaml:
	curl https://globalconfig.flashlightproxy.com/global.yaml.gz | gunzip > $@

$(CONFIG_INFO_NAME)/proxies.yaml:
	curl https://config.getiantem.org/proxies.yaml.gz | gunzip > $@

$(CONFIG_INFO_NAME).infohash: $(CONFIG_INFO_NAME).torrent
	$(TORRENT) metainfo $< infohash | cut -d : -f 1 > $@

dht-private-key:
	openssl rand -hex 32 > $@

bin/dht:
	GOBIN=`realpath bin` go install github.com/anacrolix/dht/v2/cmd/dht@latest

bin/torrent:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent@latest

bin/torrent-create:
	GOBIN=`realpath bin` go install github.com/anacrolix/torrent/cmd/torrent-create@latest

get:
	$(DHT) get `head -n 1 globalconfig.target` --salt globalconfig --extract-infohash
