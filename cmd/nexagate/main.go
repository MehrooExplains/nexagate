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
	case "user":
		if len(os.Args) < 3 {
			log.Fatal("usage: nexagate user add|disable")
		}
		fs := flag.NewFlagSet("user", flag.ExitOnError)
		configPath := fs.String("config", "/etc/nexagate/panel.json", "configuration")
		name := fs.String("name", "", "username")
		days := fs.Int("days", 30, "validity in days")
		id := fs.String("id", "", "user id")
		_ = fs.Parse(os.Args[3:])
		cfg, err := app.LoadConfigForCLI(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		store := app.NewStore(cfg.StatePath)
		switch os.Args[2] {
		case "add":
			if *name == "" {
				log.Fatal("--name is required")
			}
			u, err := store.Add(*name, *days)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("created %s (%s)\n", u.Name, u.ID)
		case "disable":
			if *id == "" {
				log.Fatal("--id is required")
			}
			if err := store.Toggle(*id); err != nil {
				log.Fatal(err)
			}
			fmt.Println("user state toggled")
		default:
			log.Fatal("unknown user command")
		}
	case "stats":
		fs := flag.NewFlagSet("stats", flag.ExitOnError)
		configPath := fs.String("config", "/etc/nexagate/panel.json", "configuration")
		_ = fs.Parse(os.Args[2:])
		cfg, err := app.LoadConfigForCLI(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("state=%s\n", cfg.StatePath)
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
		fs.StringVar(&opts.XHTTPTLSPath, "xhttp-tls-path", "", "XHTTP over TLS path")
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
	case "backup":
		fs := flag.NewFlagSet("backup", flag.ExitOnError)
		action := fs.String("action", "create", "create, restore, or list")
		output := fs.String("output", "/var/backups/nexagate.backup", "backup file")
		input := fs.String("input", "", "backup file to restore or list")
		password := fs.String("password", "", "optional encryption password")
		configPath := fs.String("config", "/etc/nexagate/panel.json", "configuration")
		statePath := fs.String("state", "/var/lib/nexagate/users.json", "user database")
		generated := fs.String("generated-dir", "/etc/nexagate/generated", "generated configs")
		_ = fs.Parse(os.Args[2:])
		switch *action {
		case "create":
			if err := app.BackupCreate(*output, *configPath, *statePath, *generated, *password); err != nil {
				log.Fatal(err)
			}
		case "restore":
			if *input == "" {
				log.Fatal("--input is required")
			}
			if err := app.BackupRestore(*input, *output, *password); err != nil {
				log.Fatal(err)
			}
		case "list":
			path := *input
			if path == "" {
				path = *output
			}
			if err := app.BackupList(path); err != nil {
				log.Fatal(err)
			}
		default:
			log.Fatal("unknown backup action")
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
  nexagate backup --action create|restore|list [options]
  nexagate dns-proxy [--listen ADDR --upstream ADDR --interface NAME]
  nexagate version`)
}
