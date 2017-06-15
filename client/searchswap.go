package client

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getlantern/flashlight/ui"
)

const (
	googleURL = "http://www.google.com/"
)

func (client *Client) searchSwap(resp *http.Response) *http.Response {
	if resp.Request.URL.String() == googleURL {
		return client.modifyGoogle(resp)
	}
	return resp
}

func (client *Client) modifyGoogle(resp *http.Response) *http.Response {
	log.Debug("Modifying Google")
	defer resp.Body.Close()

	var err error
	var r io.Reader = resp.Body
	isGZ := resp.Header.Get("Content-Encoding") == "gzip"
	if isGZ {
		r, err = gzip.NewReader(resp.Body)
		if err != nil {
			return errorResponse(resp.Request, err)
		}
	}

	body, err := ioutil.ReadAll(r)
	if err != nil {
		return errorResponse(resp.Request, err)
	}

	modified := strings.Replace(string(body), "</style>", googleCSS, 1)
	modified = strings.Replace(modified, "<script>", fmt.Sprintf(googleJS, ui.AddToken("/")+"#/plans"), 1)
	resp.Body = ioutil.NopCloser(bytes.NewReader([]byte(modified)))
	resp.Header.Del("Content-Encoding")
	return resp
}

const googleCSS = `
  #taw {
    display: none;
    font-size: 2em;
  }
</style>`

const googleJS = `<script>
(function(send, open) {
    XMLHttpRequest.prototype.send = function(data) {
      if (this.lanternQuery) {
        // This is a search, replace the paid ad results
        var query = this.lanternQuery;
        var orig = this.onreadystatechange;
        this.onreadystatechange = function() {
          orig.apply(this, arguments);
          setTimeout(function() {
            var paidAds = document.getElementById("taw");
            taw.innerHTML = "Lantern knows that you searched for " + query + ", try <a href='%v'>buying Lantern Pro instead</a>";
            taw.style.display = "block";
          }, 100);
        };
      }
      send.apply(this, arguments);
    };

    XMLHttpRequest.prototype.open = function(method, url) {
        var parsed = parseUri(url);
        if (parsed.path == "/search") {
          // Remember the query for later
          this.lanternQuery = parsed.queryKey["q"];
        }
        open.apply(this, arguments);
    };

    // parseUri 1.2.2
    // (c) Steven Levithan <stevenlevithan.com>
    // MIT License

    // parseUri 1.2.2
    // (c) Steven Levithan <stevenlevithan.com>
    // MIT License

    function parseUri (str) {
    	var	o   = parseUri.options,
    		m   = o.parser[o.strictMode ? "strict" : "loose"].exec(str),
    		uri = {},
    		i   = 14;

    	while (i--) uri[o.key[i]] = m[i] || "";

    	uri[o.q.name] = {};
    	uri[o.key[12]].replace(o.q.parser, function ($0, $1, $2) {
    		if ($1) uri[o.q.name][$1] = $2;
    	});

    	return uri;
    };

    parseUri.options = {
    	strictMode: false,
    	key: ["source","protocol","authority","userInfo","user","password","host","port","relative","path","directory","file","query","anchor"],
    	q:   {
    		name:   "queryKey",
    		parser: /(?:^|&)([^&=]*)=?([^&]*)/g
    	},
    	parser: {
    		strict: /^(?:([^:\/?#]+):)?(?:\/\/((?:(([^:@]*)(?::([^:@]*))?)?@)?([^:\/?#]*)(?::(\d*))?))?((((?:[^?#\/]*\/)*)([^?#]*))(?:\?([^#]*))?(?:#(.*))?)/,
    		loose:  /^(?:(?![^:@]+:[^:@\/]*@)([^:\/?#.]+):)?(?:\/\/)?((?:(([^:@]*)(?::([^:@]*))?)?@)?([^:\/?#]*)(?::(\d*))?)(((\/(?:[^?#](?![^?#\/]*\.[^?#\/.]+(?:[?#]|$)))*\/?)?([^?#\/]*))(?:\?([^#]*))?(?:#(.*))?)/
    	}
    };
})(XMLHttpRequest.prototype.send, XMLHttpRequest.prototype.open);
`
