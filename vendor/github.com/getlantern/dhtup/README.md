# dhtup: Go library that interfaces with DHT BEP46 resources

## Overview

dhtup uses a torrent client to fetch [BEP46](https://www.bittorrent.org/beps/bep_0046.html) resources.

To recap, BEP46 is an extension to the BitTorrent protocol that allows a single publisher to push and update torrent on the DHT. It does this by combining [BEP44](https://www.bittorrent.org/beps/bep_0044.html) -- another BitTorrent extension that allows pushing arbitrary data (not torrent) to the DHT, usually limited to 1000 bytes -- and regular torrents. In essense, when you put something on the DHT using BEP46, you're usually making a torrent and then pushing that torrent's infohash to the DHT, not the content of the torrent.

## Example of using BEP46 to put/get data on/from the DHT

Here's how to push some random data to the DHT using BEP46 and fetch it:

```

# 0. Install tools (make sure your $GOBIN path is in $PATH)
# - dht
go install github.com/anacrolix/dht/v2/cmd/dht@114cb152af7c452f70f90f3e81c41495a855a70e
# - torrent
go install github.com/anacrolix/torrent/cmd/torrent@1f6b23d995114355fa3081dcda5422ea8fa6766f
# - torrent-create
go install github.com/anacrolix/torrent/cmd/torrent-create@a319506dda5e63b4aa09dde762750689dfb1520b

# 1. Make a private key
openssl rand -hex 32 > myprivkey

# 2. Make a torrent
rm -rf mydir
mkdir mydir
echo "bunnyfoofoo" > mydir/myfile
torrent-create -n mydir > my.torrent

# 3. Get its infohash
torrent metainfo my.torrent infohash | cut -d : -f 1 > my.torrent.infohash

# 4. Put the infohash on the DHT
dht put-mutable-infohash --key `cat myprivkey` \
  --salt "salt" \
  --info-hash "`cat my.torrent.infohash`" --auto-seq | tee my.torrent.bep46target

# 5. Now, get the torrent's infohash from the DHT using the Bep46Target in "my.torrent.target"
dht get `head -n 1 my.torrent.bep46target` --salt "salt" --extract-infohash
# The result you get here will match the contents of "my.torrent.infohash"
```

## Usage Example

```
package main

import (
  "github.com/getlantern/dhtup"
	"github.com/anacrolix/missinggo/v2"
	"github.com/anacrolix/publicip"
	"github.com/anacrolix/dht/v2"
  "fmt"
  "os"
)

func main() {
    // Some random Bep46Target you wanna read its data
	  var bep46Target Krpc.ID = "..."

    // Make a DhtContext
    pubip, err := publicip.Get4(context.Background())
    dieIfErr(err)
    someConfigDir := "/tmp/whatever"
    dhtupContext, err := dhtup.NewContext(ipv4, someConfigDir)
    dieIfErr(err)

    // Declare a new dhtup.Resource
    res := dhtup.NewResource(dhtup.ResourceInput{
      DhtTarget:  bep46Target,
      DhtContext: dhtupContext,
      // Your file path. In the "Example" case above, that would be "myfile"
      FilePath: "your file path",
      WebSeedUrls: []string{...},
      Salt: []byte("whatever"),
      MetainfoUrls: []string{...},
    })
    reader, _, err := res.Open(context.TODO())
    dieIfErr(err)
    defer reader.Close()

    // Start reading the data (i.e., downloading the torrent)
    ctxReader := missinggo.ContextedReader{R: reader, Ctx: context.TODO()}
    fd, err := os.CreateTemp("", "whatever")
    dieIfErr(err)
    defer fd.Close()
    // This operation is blocking: it'll start basically downloading the torrent at this point
    _, err = io.Copy(fd, ctxReader)
    dieIfErr(err)

    fmt.Printf("File downloaded\n")
}
```
