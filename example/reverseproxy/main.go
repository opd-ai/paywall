package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/opd-ai/paywall"
	reverseproxy "github.com/opd-ai/paywall/example/reverseproxy/proxy"
	wileedot "github.com/opd-ai/wileedot"
	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
)

// flags: -target http://locaPlhost:3000
// flags: -protected-path /protected
// flags: -price-in-btc 0.0001
// flags: -price-in-xmr 0.01
// flags: -payment-timeout 10m
// flags: -min-confirmations 1
// flags: -testnet
// flags: -hostname localhost
// flags: -port 8080
// flags: -letsencrypt false
// flags: -email ""
// flags: -cert-dir ./
var target = flag.String("target", "http://localhost:3000", "target server URL")
var protectedPath = flag.String("protected-path", "/protected", "protected path requiring payment")
var priceInBTC = flag.Float64("price-in-btc", 0.0001, "price in BTC for access")
var priceInXMR = flag.Float64("price-in-xmr", 0.01, "price in XMR for access")
var paymentTimeout = flag.Duration("payment-timeout", 10*time.Minute, "payment timeout duration")
var minConfirmations = flag.Int("min-confirmations", 1, "minimum blockchain confirmations required")
var testnet = flag.Bool("testnet", false, "use Bitcoin testnet")
var hostname = flag.String("hostname", "localhost", "hostname for the server")
var port = flag.String("port", "8080", "port for the server")
var letsencrypt = flag.Bool("letsencrypt", false, "use Let's Encrypt for HTTPS")
var email = flag.String("email", "", "email for Let's Encrypt certificate")
var certDir = flag.String("cert-dir", wd(), "directory for Let's Encrypt certificates")
var tokens = flag.Uint64("tokens", 15, "number of tokens allowed per interval")
var interval = flag.Duration("interval", 1*time.Minute, "interval until tokens reset")

func wd() string {
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	return wd
}

func main() {
	// parse flags
	flag.Parse()
	// create a new paywall configuration
	config := paywall.Config{
		PriceInBTC:       *priceInBTC,
		PriceInXMR:       *priceInXMR,
		PaymentTimeout:   *paymentTimeout,
		MinConfirmations: *minConfirmations,
		TestNet:          *testnet,
	}
	// create a new paywall instance
	paywall, err := paywall.NewPaywall(config)
	if err != nil {
		log.Fatal(err)
	}
	proxy, err := reverseproxy.NewProxy(*target, paywall)
	if err != nil {
		log.Fatal(err)
	}
	if *protectedPath != "" {
		proxy.ProtectedPath = *protectedPath
	}
	var listener net.Listener
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   *tokens,
		Interval: *interval,
	})
	if err != nil {
		log.Fatal(err)
	}
	limiter, err := httplimit.NewMiddleware(store, httplimit.IPKeyFunc())
	if err != nil {
		log.Fatal(err)
	}
	if *letsencrypt {
		cfg := wileedot.Config{
			Domain:         *hostname,
			AllowedDomains: []string{*hostname},
			CertDir:        *certDir,
			Email:          *email,
		}
		listener, err = wileedot.New(cfg)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		listenaAddr := net.JoinHostPort(*hostname, *port)
		listener, err = net.Listen("tcp", listenaAddr)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := http.Serve(listener, limiter.Handle(proxy)); err != nil {
		log.Fatal(err)
	}

}
