/*
Copyright © 2026 Nehemia
*/
package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"coolify-tools/internal/harden"
	internalssh "coolify-tools/internal/ssh"
	"coolify-tools/internal/utils"
)

var (
	hardenAllowPorts          []string
	hardenTailscaleIface      string
	hardenProxyContainer      string
	hardenMaxAuthTries        int
	hardenClientAliveInterval int
	hardenClientAliveCountMax int
	hardenAllowPasswordAuth   bool
	hardenFail2banMaxRetry    int
	hardenFail2banFindTime    int
	hardenFail2banBanTime     int
	hardenSkipSSH             bool
	hardenSkipFail2ban        bool
	hardenSkipUFW             bool
	hardenSkipUFWDocker       bool
	hardenDryRun              bool
	hardenYes                 bool
)

var hardenCmd = &cobra.Command{
	Use:   "harden",
	Short: "Locks down a freshly installed Coolify server (SSH + firewall)",
	Long: `Connects to a Coolify host and applies the post-install security checklist:
key-only SSH with fail2ban, and a default-deny UFW firewall (including ufw-docker
so Docker-published ports actually respect UFW rules).

This can lock you out of the server if something goes wrong. Keep a second SSH
session open to the target host before running this for the first time.

Example:
  coolify-tools harden server.example.com ~/.ssh/id_ed25519`,
	Args: cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		hostname := args[0]
		sshKey := args[1]

		utils.HandleErr("harden failed", runHarden(hostname, sshKey))
	},
}

func runHarden(hostname, sshKey string) error {
	allowPorts := append([]string{}, hardenAllowPorts...)
	sshPortRule := sshPort + "/tcp"
	if !contains(allowPorts, sshPortRule) {
		allowPorts = append([]string{sshPortRule}, allowPorts...)
	}

	cfg := harden.Config{
		AllowPorts:          allowPorts,
		TailscaleIface:      hardenTailscaleIface,
		ProxyContainer:      hardenProxyContainer,
		MaxAuthTries:        hardenMaxAuthTries,
		ClientAliveInterval: hardenClientAliveInterval,
		ClientAliveCountMax: hardenClientAliveCountMax,
		AllowPasswordAuth:   hardenAllowPasswordAuth,
		Fail2banMaxRetry:    hardenFail2banMaxRetry,
		Fail2banFindTime:    hardenFail2banFindTime,
		Fail2banBanTime:     hardenFail2banBanTime,
		SkipSSH:             hardenSkipSSH,
		SkipFail2ban:        hardenSkipFail2ban,
		SkipUFW:             hardenSkipUFW,
		SkipUFWDocker:       hardenSkipUFWDocker,
	}

	script := harden.BuildScript(cfg)

	if hardenDryRun {
		fmt.Println(script)
		return nil
	}

	fmt.Printf("About to harden %s:\n", hostname)
	fmt.Printf("  - Allow ports: %s\n", strings.Join(allowPorts, ", "))
	if cfg.TailscaleIface != "" {
		fmt.Printf("  - Tailscale interface: %s (allowed unrestricted)\n", cfg.TailscaleIface)
	}
	if !cfg.SkipSSH && !cfg.AllowPasswordAuth {
		fmt.Println("  - SSH password authentication will be DISABLED")
	}
	if !cfg.SkipUFW {
		fmt.Println("  - UFW will default-deny incoming traffic")
	}
	fmt.Println("Keep a second SSH session open to this host until you've confirmed you can still log in.")

	if !hardenYes && !utils.YesNoPrompt("Proceed?", false) {
		return fmt.Errorf("aborted by user")
	}

	client, _ := internalssh.EstablishConnection(username, hostname, sshKey, sshPort, passphrase)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create ssh session: %v", err)
	}
	defer session.Close()

	session.Stdin = strings.NewReader(script)
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if err := session.Run("bash -s"); err != nil {
		return fmt.Errorf("hardening script failed: %v", err)
	}

	log.Println("Hardening complete for", hostname)
	return nil
}

func contains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func init() {
	hardenCmd.Flags().StringSliceVar(&hardenAllowPorts, "allow-port", harden.DefaultAllowPorts, "Additional ports to allow through UFW (e.g. 8080/tcp), repeatable")
	hardenCmd.Flags().StringVar(&hardenTailscaleIface, "tailscale-iface", "", "Tailscale interface to allow unrestricted traffic on (e.g. tailscale0); empty disables")
	hardenCmd.Flags().StringVar(&hardenProxyContainer, "proxy-container", harden.DefaultProxyContainer, "Name of Coolify's proxy container to allow through ufw-docker on 80/443")

	hardenCmd.Flags().IntVar(&hardenMaxAuthTries, "max-auth-tries", 3, "sshd MaxAuthTries")
	hardenCmd.Flags().IntVar(&hardenClientAliveInterval, "client-alive-interval", 300, "sshd ClientAliveInterval (seconds)")
	hardenCmd.Flags().IntVar(&hardenClientAliveCountMax, "client-alive-count-max", 2, "sshd ClientAliveCountMax")
	hardenCmd.Flags().BoolVar(&hardenAllowPasswordAuth, "allow-password-auth", false, "Do not disable SSH password authentication")

	hardenCmd.Flags().IntVar(&hardenFail2banMaxRetry, "fail2ban-maxretry", 3, "fail2ban sshd jail maxretry")
	hardenCmd.Flags().IntVar(&hardenFail2banFindTime, "fail2ban-findtime", 600, "fail2ban sshd jail findtime (seconds)")
	hardenCmd.Flags().IntVar(&hardenFail2banBanTime, "fail2ban-bantime", 3600, "fail2ban sshd jail bantime (seconds)")

	hardenCmd.Flags().BoolVar(&hardenSkipSSH, "skip-ssh", false, "Skip SSH daemon hardening")
	hardenCmd.Flags().BoolVar(&hardenSkipFail2ban, "skip-fail2ban", false, "Skip fail2ban install/config")
	hardenCmd.Flags().BoolVar(&hardenSkipUFW, "skip-ufw", false, "Skip UFW firewall configuration")
	hardenCmd.Flags().BoolVar(&hardenSkipUFWDocker, "skip-ufw-docker", false, "Skip ufw-docker install (leaves Docker-published ports bypassing UFW)")

	hardenCmd.Flags().BoolVar(&hardenDryRun, "dry-run", false, "Print the generated script instead of executing it")
	hardenCmd.Flags().BoolVarP(&hardenYes, "yes", "y", false, "Skip the confirmation prompt")

	rootCmd.AddCommand(hardenCmd)
}
