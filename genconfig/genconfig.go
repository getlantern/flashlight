package main

import (
	"io/ioutil"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/getlantern/flashlight/client"
	"github.com/getlantern/golog"
	"github.com/getlantern/keyman"
	"gopkg.in/getlantern/tlsdialer.v2"
)

const (
	numberOfWorkers = 50
)

var (
	log = golog.LoggerFor("genconfig")

	masqueradesTmpl string
	yamlTmpl        string

	domainsCh     = make(chan string)
	masqueradesCh = make(chan *client.Masquerade)
	masquerades   = make([]*client.Masquerade, 0)
	wg            sync.WaitGroup

	model map[string]interface{}
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Please specify a file with a list of domains to use")
	}
	domainsFile := os.Args[1]
	domains, err := ioutil.ReadFile(domainsFile)
	if err != nil {
		log.Fatalf("Unable to read domains file at %s: %s", domainsFile, err)
	}

	masqueradesTmpl = loadTemplate("masquerades.go.tmpl")
	yamlTmpl = loadTemplate("cloud.yaml.tmpl")

	go feedDomains(string(domains))
	coalesceMasquerades()
	buildModel()
	generateTemplate(masqueradesTmpl, "../config/masquerades.go")
	generateTemplate(yamlTmpl, "cloud.yaml")
}

func loadTemplate(name string) string {
	bytes, err := ioutil.ReadFile(name)
	if err != nil {
		log.Fatalf("Unable to load template %s: %s", name, err)
	}
	return string(bytes)
}

func feedDomains(domains string) {
	wg.Add(numberOfWorkers)
	for i := 0; i < numberOfWorkers; i++ {
		go grabCerts()
	}

	for _, domain := range strings.Split(domains, "\n") {
		domainsCh <- domain
	}
	close(domainsCh)
	wg.Wait()
	close(masqueradesCh)
}

// grabCerts grabs certificates for the domains received on domainsCh and sends
// *client.Masquerades to masqueradesCh.
func grabCerts() {
	defer wg.Done()

	for domain := range domainsCh {
		log.Tracef("Grabbing certs for domain: %s", domain)
		cwt, err := tlsdialer.DialForTimings(&net.Dialer{
			Timeout: 10 * time.Second,
		}, "tcp", domain+":443", false, nil)
		if err != nil {
			log.Errorf("Unable to dial domain %s: %s", domain, err)
			continue
		}
		cwt.Conn.Close()
		chain := cwt.VerifiedChains[0]
		rootCA := chain[len(chain)-1]
		rootCert, err := keyman.LoadCertificateFromX509(rootCA)
		if err != nil {
			log.Errorf("Unablet to load keyman certificate: %s", err)
			continue
		}
		masqueradesCh <- &client.Masquerade{
			Domain: domain,
			RootCA: strings.Replace(string(rootCert.PEMEncoded()), "\n", "\\n", -1),
		}
	}
}

func coalesceMasquerades() {
	for masquerade := range masqueradesCh {
		masquerades = append(masquerades, masquerade)
	}
}

func buildModel() {
	sort.Sort(ByDomain(masquerades))
	model = map[string]interface{}{
		"masquerades": masquerades,
	}
}

func generateTemplate(tmplString string, filename string) {
	tmpl, err := template.New(filename).Parse(tmplString)
	if err != nil {
		log.Errorf("Unable to parse template: %s", err)
		return
	}
	out, err := os.Create(filename)
	if err != nil {
		log.Errorf("Unable to create %s: %s", filename, err)
		return
	}
	defer out.Close()
	err = tmpl.Execute(out, model)
	if err != nil {
		log.Errorf("Unable to generate %s: %s", filename, err)
	}
}

type ByDomain []*client.Masquerade

func (a ByDomain) Len() int           { return len(a) }
func (a ByDomain) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDomain) Less(i, j int) bool { return a[i].Domain < a[j].Domain }
