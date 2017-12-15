package ipfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"

	"github.com/getlantern/go-ipfs/core"
	"github.com/getlantern/go-ipfs/core/coreunix"
	"github.com/getlantern/go-ipfs/path"
	"github.com/getlantern/go-ipfs/repo/fsrepo"
	uio "github.com/getlantern/go-ipfs/unixfs/io"
	node "github.com/ipfs/go-ipld-format"
	crypto "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
)

type IpfsNode struct {
	node   *core.IpfsNode
	pk     crypto.PrivKey
	ctx    context.Context
	cancel context.CancelFunc
}

type IpnsEntry struct {
	Name  string
	Value string
}

func Start(repoDir string, pkfile string) (*IpfsNode, error) {
	r, err := fsrepo.Open(repoDir)
	if err != nil {
		return nil, err
	}

	var pk crypto.PrivKey
	if pkfile != "" {
		pk, err = GenKeyIfNotExists(pkfile)
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	cfg := &core.BuildCfg{
		Repo:   r,
		Online: true,
	}

	nd, err := core.NewNode(ctx, cfg)

	if err != nil {
		return nil, err
	}
	return &IpfsNode{nd, pk, ctx, cancel}, nil
}

func (node *IpfsNode) Stop() {
	node.cancel()
}

func (node *IpfsNode) Add(content string, name string) (path string, dNode node.Node, err error) {
	return coreunix.AddWrapped(node.node, strings.NewReader(content), name)
}

func (node *IpfsNode) AddFile(fileName string, name string) (path string, dNode node.Node, err error) {
	file, err := os.Open(fileName)
	if err != nil {
		return "", nil, err
	}
	return coreunix.AddWrapped(node.node, file, name)
}

func (node *IpfsNode) Get(pt string) (string, error) {
	p := path.Path(pt)
	dn, err := core.Resolve(node.ctx, node.node.Namesys, node.node.Resolver, p)
	if err != nil {
		return "", err
	}

	reader, err := uio.NewDagReader(node.ctx, dn, node.node.DAG)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	_, err = io.Copy(&buf, reader)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (node *IpfsNode) Publish(p string) (string, error) {
	ref := path.Path(p)
	k := node.node.PrivateKey
	if node.pk != nil {
		k = node.pk
	}
	err := node.node.Namesys.Publish(node.ctx, k, ref)
	if err != nil {
		return "", err
	}

	pid, err := peer.IDFromPrivateKey(k)
	if err != nil {
		return "", err
	}

	return pid.Pretty(), nil
}

func (node *IpfsNode) Resolve(name string) (string, error) {
	p, err := node.node.Namesys.ResolveN(node.ctx, name, 1)
	if err != nil {
		return "", err
	}

	return p.String(), nil
}
