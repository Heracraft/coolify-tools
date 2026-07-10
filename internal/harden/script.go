package harden

import (
	"fmt"
	"strings"
)

var DefaultAllowPorts = []string{"80/tcp", "443/tcp", "8000/tcp", "6001/tcp", "6002/tcp"}

const DefaultProxyContainer = "coolify-proxy"

type Config struct {
	AllowPorts          []string
	TailscaleIface      string
	ProxyContainer      string
	MaxAuthTries        int
	ClientAliveInterval int
	ClientAliveCountMax int
	AllowPasswordAuth   bool
	Fail2banMaxRetry    int
	Fail2banFindTime    int
	Fail2banBanTime     int
	SkipSSH             bool
	SkipFail2ban        bool
	SkipUFW             bool
	SkipUFWDocker       bool
}

// BuildScript renders an idempotent bash script that applies the hardening
// checklist. Sections run least-risky-first (firewall, then fail2ban, then
// SSH config) so a partial run never locks out the key-based session used
// to deliver it.
func BuildScript(cfg Config) string {
	var b strings.Builder

	b.WriteString("#!/usr/bin/env bash\nset -euo pipefail\n\n")

	if !cfg.SkipUFW {
		b.WriteString("echo '==> Configuring UFW firewall'\n")
		b.WriteString("ufw --force default deny incoming\n")
		b.WriteString("ufw --force default allow outgoing\n")
		for _, port := range cfg.AllowPorts {
			fmt.Fprintf(&b, "ufw allow %s comment 'coolify-tools harden'\n", port)
		}
		if cfg.TailscaleIface != "" {
			fmt.Fprintf(&b, "ufw allow in on %s\n", cfg.TailscaleIface)
			fmt.Fprintf(&b, "ufw allow out on %s\n", cfg.TailscaleIface)
		}
		b.WriteString("ufw --force enable\n\n")

		if !cfg.SkipUFWDocker {
			b.WriteString("echo '==> Installing ufw-docker'\n")
			b.WriteString("if [ ! -x /usr/local/bin/ufw-docker ]; then\n")
			b.WriteString("  curl -fsSL -o /usr/local/bin/ufw-docker https://github.com/chaifeng/ufw-docker/raw/master/ufw-docker\n")
			b.WriteString("  chmod +x /usr/local/bin/ufw-docker\n")
			b.WriteString("fi\n")
			b.WriteString("ufw-docker install\n")
			b.WriteString("systemctl restart ufw\n")
			if cfg.TailscaleIface != "" {
				fmt.Fprintf(&b, "ufw route allow in on %s to any\n", cfg.TailscaleIface)
			}
			b.WriteString("ufw reload\n")

			if cfg.ProxyContainer != "" {
				fmt.Fprintf(&b, "ufw-docker allow %s 80 || true\n", cfg.ProxyContainer)
				fmt.Fprintf(&b, "ufw-docker allow %s 443 || true\n\n", cfg.ProxyContainer)
			} else {
				b.WriteString("\n")
			}
		}
	}

	if !cfg.SkipFail2ban {
		b.WriteString("echo '==> Installing fail2ban'\n")
		b.WriteString("apt-get update -y\n")
		b.WriteString("apt-get install -y fail2ban\n")
		b.WriteString("mkdir -p /etc/fail2ban/jail.d\n")
		fmt.Fprintf(&b, `cat > /etc/fail2ban/jail.d/99-coolify-tools.local <<'EOF'
[sshd]
enabled = true
filter = sshd
logpath = /var/log/auth.log
maxretry = %d
findtime = %d
bantime = %d
banaction = iptables-multiport

[sshd-ddos]
enabled = true
filter = sshd-ddos
logpath = /var/log/auth.log
maxretry = %d
findtime = 60
bantime = %d
EOF
`, cfg.Fail2banMaxRetry, cfg.Fail2banFindTime, cfg.Fail2banBanTime, cfg.Fail2banMaxRetry+2, cfg.Fail2banBanTime*2)
		b.WriteString("systemctl enable fail2ban\n")
		b.WriteString("systemctl restart fail2ban\n\n")
	}

	if !cfg.SkipSSH {
		b.WriteString("echo '==> Hardening SSH daemon config'\n")
		b.WriteString("mkdir -p /etc/ssh/sshd_config.d\n")
		b.WriteString("grep -q '^Include /etc/ssh/sshd_config.d/\\*\\.conf' /etc/ssh/sshd_config || sed -i '1i Include /etc/ssh/sshd_config.d/*.conf' /etc/ssh/sshd_config\n")

		passwordAuth := "no"
		if cfg.AllowPasswordAuth {
			passwordAuth = "yes"
		}

		fmt.Fprintf(&b, `cat > /etc/ssh/sshd_config.d/99-coolify-tools-hardening.conf <<'EOF'
PasswordAuthentication %s
PubkeyAuthentication yes
PermitRootLogin prohibit-password
ChallengeResponseAuthentication no
UsePAM no
MaxAuthTries %d
ClientAliveInterval %d
ClientAliveCountMax %d
EOF
`, passwordAuth, cfg.MaxAuthTries, cfg.ClientAliveInterval, cfg.ClientAliveCountMax)

		b.WriteString("sshd -t\n")
		b.WriteString("systemctl reload ssh 2>/dev/null || systemctl reload sshd\n\n")
	}

	b.WriteString("echo '==> Hardening complete'\n")

	return b.String()
}
