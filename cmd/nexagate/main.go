package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/MehrooExplains/nexagate/internal/app"
)

var version = "dev"

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		configPath := fs.String("config", "/etc/nexagate/panel.json", "panel configuration")
		_ = fs.Parse(os.Args[2:])
		if err := app.Serve(*configPath, version); err != nil {
			log.Fatal(err)
		}
	case "init":
		fs := flag.NewFlagSet("init", flag.ExitOnError)
		opts := app.InitOptions{}
		fs.StringVar(&opts.ConfigPath, "config", "/etc/nexagate/panel.json", "configuration output")
		fs.StringVar(&opts.StatePath, "state", "/var/lib/nexagate/users.json", "user database")
		fs.StringVar(&opts.GeneratedDir, "generated-dir", "/etc/nexagate/generated", "generated service configs")
		fs.StringVar(&opts.Listen, "listen", "127.0.0.1:9080", "panel listen address")
		fs.StringVar(&opts.PublicHost, "host", "", "public domain or IP")
		fs.StringVar(&opts.CertName, "cert-name", "", "CertDuo certificate name")
		fs.StringVar(&opts.ACMEWebroot, "webroot", "/var/www/html", "CertDuo ACME webroot")
		fs.StringVar(&opts.AdminPassword, "admin-password", "", "initial administrator password")
		fs.StringVar(&opts.AdminPasswordFile, "admin-password-file", "", "file containing the initial administrator password")
		fs.StringVar(&opts.RealityPrivateKey, "reality-private-key", "", "X25519 private key")
		fs.StringVar(&opts.RealityPrivateKeyFile, "reality-private-key-file", "", "file containing the X25519 private key")
		fs.StringVar(&opts.RealityPublicKey, "reality-public-key", "", "X25519 public key")
		fs.StringVar(&opts.RealityTarget, "reality-target", "www.microsoft.com", "REALITY camouflage target")
		fs.StringVar(&opts.RealityShortID, "reality-short-id", "", "REALITY short ID")
		fs.StringVar(&opts.HysteriaObfs, "hysteria-obfs", "", "Hysteria2 Salamander password")
		fs.StringVar(&opts.XHTTPPath, "xhttp-path", "", "XHTTP path")
		fs.StringVar(&opts.WebSocketPath, "websocket-path", "", "WebSocket path")
		_ = fs.Parse(os.Args[2:])
		if err := app.Initialize(opts); err != nil {
			log.Fatal(err)
		}
	case "render":
		fs := flag.NewFlagSet("render", flag.ExitOnError)
		configPath := fs.String("config", "/etc/nexagate/panel.json", "panel configuration")
		_ = fs.Parse(os.Args[2:])
		if err := app.RenderFromFile(*configPath); err != nil {
			log.Fatal(err)
		}
	case "doctor":
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		configPath := fs.String("config", "/etc/nexagate/panel.json", "panel configuration")
		_ = fs.Parse(os.Args[2:])
		if err := app.Doctor(*configPath, os.Stdout); err != nil {
			log.Fatal(err)
		}
	case "dns-proxy":
		fs := flag.NewFlagSet("dns-proxy", flag.ExitOnError)
		listen := fs.String("listen", "127.0.0.1:1053", "UDP listen address")
		upstream := fs.String("upstream", "1.1.1.1:53", "upstream DNS server")
		device := fs.String("interface", "warp0", "required outbound interface")
		_ = fs.Parse(os.Args[2:])
		if err := app.RunDNSProxy(*listen, *upstream, *device); err != nil {
			log.Fatal(err)
		}
	case "version", "--version", "-v":
		fmt.Println(version)
	case "help", "--help", "-h":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `NexaGate - multi-inbound, dual-egress gateway

Usage:
  nexagate serve  [--config FILE]
  nexagate init   [options]
  nexagate render [--config FILE]
  nexagate doctor [--config FILE]
  nexagate dns-proxy [--listen ADDR --upstream ADDR --interface NAME]
  nexagate version`)
}
