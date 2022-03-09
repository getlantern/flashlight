publish: globalconfig.infohash bin/dht dht-private-key
	bin/dht put-mutable-infohash --key `cat dht-private-key` --salt globalconfig --info-hash `cat globalconfig.infohash` --seq '$(SEQ)' | tee globalconfig.target

globalconfig.torrent: root root/global.yaml
	torrent-create root > $@

root:
	mkdir $@

root/global.yaml:
	curl https://globalconfig.flashlightproxy.com/global.yaml.gz | gunzip > $@

globalconfig.infohash: globalconfig.torrent
	torrent metainfo $< infohash | cut -d : -f 1 > $@

dht-private-key:
	openssl rand -hex 32 > $@

bin/dht:
	GOBIN=`realpath bin` go install github.com/anacrolix/dht/v2/cmd/dht@latest

get:
	bin/dht get `head -n 1 globalconfig.target` --salt globalconfig --extract-infohash
