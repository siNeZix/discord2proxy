package main

import (
	"flag"
	"fmt"
	"os"

	"discord-szx/internal/config"
	"discord-szx/internal/deploy"
	"discord-szx/internal/discord"
	"discord-szx/internal/proxy"
)

func main() {
	cfg := config.Default()

	proxyPort := flag.Int("proxy-port", 0, "Override proxy port (0 = auto-detect)")
	proxyHost := flag.String("proxy-host", "127.0.0.1", "Override proxy host")
	discordPath := flag.String("discord-path", "", "Override Discord installation path")
	channel := flag.String("channel", "", "Select Discord channel: stable|ptb|canary|development (alias: dev)")
	dryRun := flag.Bool("dry-run", false, "Show what would be done without executing")
	force := flag.Bool("force", false, "Deploy even if Discord is running")
	noVerify := flag.Bool("no-verify", false, "Skip post-deploy verification")
	flag.Parse()

	fmt.Printf("%s — Discord proxy deployer\n", config.Title())
	fmt.Println()

	// --- Proxy Detection ---
	fmt.Println("[1/4] Detecting SOCKS5 proxy...")
	var proxyInfo *proxy.ProxyInfo
	var err error

	if *proxyPort != 0 {
		fmt.Printf("  Using manual proxy: %s:%d\n", *proxyHost, *proxyPort)
		proxyInfo, err = proxy.VerifyProxy(*proxyHost, *proxyPort)
		if err != nil {
			if *force {
				fmt.Fprintf(os.Stderr, "  WARNING: %v (continuing due to -force)\n", err)
				proxyInfo = &proxy.ProxyInfo{
					Host:   *proxyHost,
					Port:   *proxyPort,
					Source: "manual",
					Alive:  false,
				}
			} else {
				fmt.Fprintf(os.Stderr, "ERROR: %v (use -force to deploy anyway)\n", err)
				os.Exit(1)
			}
		} else {
			proxyInfo.Source = "manual"
		}
	} else {
		proxyInfo, err = proxy.DetectBestProxy(*proxyHost, cfg.ProxyPorts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("  Found: %s:%d (%s)\n", proxyInfo.Host, proxyInfo.Port, proxyInfo.Source)

	// --- Discord Finder ---
	fmt.Println("[2/4] Locating Discord installation...")
	var install *discord.DiscordInstall

	if *discordPath != "" {
		install, err = discord.FindDiscordByPath(*discordPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(2)
		}
	} else if *channel != "" {
		install, err = discord.FindDiscordByChannel(cfg, *channel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(2)
		}
	} else {
		install, err = discord.FindPrimaryDiscord(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(2)
		}
	}
	fmt.Printf("  Found: %s (channel=%s, version=%s)\n", install.AppDir, install.Channel, install.Version)

	// --- Deploy ---
	fmt.Println("[3/4] Deploying files...")
	d := deploy.New(cfg, *dryRun, *force)
	if err := d.Deploy(install, proxyInfo); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: deploy failed: %v\n", err)
		os.Exit(3)
	}

	// --- Verify ---
	if !*noVerify && !*dryRun {
		fmt.Println("[4/4] Verifying deployment...")
		if err := d.Verify(install); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: verification failed: %v\n", err)
			os.Exit(4)
		}
	} else {
		fmt.Println("[4/4] Verification skipped")
	}

	fmt.Println()
	fmt.Println("Done! Restart Discord to apply the proxy.")
}
