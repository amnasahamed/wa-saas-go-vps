#!/usr/bin/env bash
#
# One-time VPS preparation for the NiosEngine stack.
# Tested on Ubuntu 22.04 / 24.04 LTS. Run as root or with sudo:
#
#   sudo bash deploy/vps/bootstrap.sh
#
# Idempotent: safe to re-run.

set -euo pipefail

SSH_PORT="${SSH_PORT:-22}"
SWAP_SIZE="${SWAP_SIZE:-2G}"
DEPLOY_USER="${DEPLOY_USER:-deploy}"

log() { printf '\n\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33mwarning:\033[0m %s\n' "$*" >&2; }

if [[ $EUID -ne 0 ]]; then
	echo "error: run as root (sudo bash $0)" >&2
	exit 1
fi

# ---- base packages -------------------------------------------------------
log "Installing base packages"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq \
	ca-certificates curl gnupg git ufw fail2ban \
	unattended-upgrades postgresql-client

# ---- Docker engine + compose plugin --------------------------------------
if command -v docker >/dev/null 2>&1; then
	log "Docker already installed: $(docker --version)"
else
	log "Installing Docker from the official repository"
	install -m 0755 -d /etc/apt/keyrings
	curl -fsSL https://download.docker.com/linux/ubuntu/gpg |
		gpg --dearmor -o /etc/apt/keyrings/docker.gpg
	chmod a+r /etc/apt/keyrings/docker.gpg

	# shellcheck source=/dev/null
	. /etc/os-release
	echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu ${VERSION_CODENAME} stable" \
		>/etc/apt/sources.list.d/docker.list

	apt-get update -qq
	apt-get install -y -qq \
		docker-ce docker-ce-cli containerd.io \
		docker-buildx-plugin docker-compose-plugin
fi

systemctl enable --now docker

# ---- swap ----------------------------------------------------------------
# The Next.js build is memory hungry. On a 1-2GB VPS it gets OOM-killed
# without swap.
if swapon --show | grep -q '/swapfile'; then
	log "Swap already configured"
else
	log "Creating ${SWAP_SIZE} swapfile"
	fallocate -l "$SWAP_SIZE" /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048
	chmod 600 /swapfile
	mkswap /swapfile >/dev/null
	swapon /swapfile
	grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
	# Prefer RAM but allow spillover during builds.
	sysctl -qw vm.swappiness=10
	grep -q 'vm.swappiness' /etc/sysctl.conf || echo 'vm.swappiness=10' >>/etc/sysctl.conf
fi

# ---- firewall ------------------------------------------------------------
# Only SSH + HTTP/HTTPS. Postgres, Redis, and both gRPC ports stay on the
# Docker network and are never published to the host.
log "Configuring ufw"
ufw --force reset >/dev/null
ufw default deny incoming >/dev/null
ufw default allow outgoing >/dev/null
ufw allow "${SSH_PORT}/tcp" comment 'SSH' >/dev/null
ufw allow 80/tcp comment 'HTTP (ACME + redirect)' >/dev/null
ufw allow 443/tcp comment 'HTTPS' >/dev/null
ufw allow 443/udp comment 'HTTP/3' >/dev/null
ufw --force enable >/dev/null
ufw status verbose

warn "Docker publishes ports by bypassing ufw's INPUT chain. This stack only"
warn "publishes 80/443 via Caddy, so that is fine — but do not add host port"
warn "mappings for postgres/redis and assume ufw will block them."

# ---- automatic security updates -----------------------------------------
log "Enabling unattended-upgrades"
systemctl enable --now unattended-upgrades || warn "could not enable unattended-upgrades"

# ---- fail2ban for SSH ----------------------------------------------------
log "Enabling fail2ban"
systemctl enable --now fail2ban || warn "could not enable fail2ban"

# ---- deploy user ---------------------------------------------------------
if id "$DEPLOY_USER" >/dev/null 2>&1; then
	log "User ${DEPLOY_USER} already exists"
else
	log "Creating ${DEPLOY_USER} user in the docker group"
	adduser --disabled-password --gecos "" "$DEPLOY_USER"
	usermod -aG docker "$DEPLOY_USER"
	if [[ -f /root/.ssh/authorized_keys ]]; then
		install -d -m 700 -o "$DEPLOY_USER" -g "$DEPLOY_USER" "/home/${DEPLOY_USER}/.ssh"
		install -m 600 -o "$DEPLOY_USER" -g "$DEPLOY_USER" \
			/root/.ssh/authorized_keys "/home/${DEPLOY_USER}/.ssh/authorized_keys"
		log "Copied root's authorized_keys to ${DEPLOY_USER}"
	else
		warn "no /root/.ssh/authorized_keys — add a key for ${DEPLOY_USER} before disabling root login"
	fi
fi

log "Done. Next steps:"
cat <<EOF

  1. Point an A record for your domain at this server's public IP.
  2. su - ${DEPLOY_USER}
  3. git clone <your-private-repo> niosengine && cd niosengine
  4. cp .env.prod.example .env.prod && \$EDITOR .env.prod
     - set APP_DOMAIN, ACME_EMAIL
     - generate JWT_SECRET:       openssl rand -base64 48
     - generate POSTGRES_PASSWORD: openssl rand -base64 32 | tr -d '/+='
     - paste that password into DATABASE_URL too
  5. make prod-up
  6. Install the nightly backup:
     sudo cp deploy/vps/backup.sh /usr/local/bin/niosengine-backup
     sudo chmod +x /usr/local/bin/niosengine-backup
     ( crontab -l 2>/dev/null; echo "17 3 * * * /usr/local/bin/niosengine-backup" ) | crontab -

EOF
