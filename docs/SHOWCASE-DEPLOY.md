# Hosting the showcase: Vercel and an Oracle Cloud ARM machine

This is for showing the project to people. It is a demo, not a service for customers: the chaos switch and the demo bots are on, on purpose.

```
 visitor ──► Vercel (the frontend/ folder, static)
                │  fetch + WebSocket
                ▼
 https://api.<your-ip>.nip.io  ──►  Caddy (TLS) ──► api-1 ──► biddingsvc, bidworker,
                                       Oracle ARM VM            Postgres, Redis,
                                                                Elasticsearch, Kafka
```

Without the backend the frontend still works: it falls back to the in-browser demo engine, clearly labelled as simulated. So the order below is safe: Vercel first, backend second.

## 1. The backend on an Oracle Cloud ARM machine

**Create the machine**

1. Oracle Cloud, Compute, Create instance. Image: Ubuntu 24.04 (aarch64). Shape: **VM.Standard.A1.Flex**, 2 OCPU and 12 GB RAM is plenty (the free allowance is 4 OCPU and 24 GB). Add your SSH key. Note the public IP.
2. In the instance's subnet, Security List: add ingress rules for TCP **80** and **443** from `0.0.0.0/0`. (22 is already open.)
3. SSH in and let the machine's own firewall through. Oracle's Ubuntu images ship iptables rules that block 80 and 443:

```bash
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 80  -j ACCEPT
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save
```

**Install Docker and fetch the code**

```bash
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER && newgrp docker
git clone https://github.com/ayush1388/auctionEngine.git && cd auctionEngine
```

**Configure it** (`deploy/.env` is gitignored, so secrets never reach git):

```bash
cp deploy/.env.example deploy/.env
for k in POSTGRES_PASSWORD JWT_SECRET INTERNAL_TOKEN GRAFANA_ADMIN_PASSWORD; do
  sed -i "s|^$k=.*|$k=$(openssl rand -hex 32)|" deploy/.env
done
```

Then edit `deploy/.env` and set:

| Key | Value |
|---|---|
| `SITE_ADDRESS` | `api.<IP with dashes>.nip.io`, for example `api.130-61-10-20.nip.io`. Caddy gets a free Let's Encrypt certificate for it. Or use your own domain with an A record. |
| `PUBLIC_URL` | `https://` plus the same name |
| `CORS_ALLOWED_ORIGINS` | your Vercel URL, for example `https://marque-demo.vercel.app` (add it after step 2) |
| `CHAOS_ENABLED` | `true` |
| `DEMO_BOTS_ENABLED` | `true` |
| `APP_BASE_URL` | your site's address, for example `https://auctionengine-frontend.vercel.app` (the password reset email links to it; the email only arrives if the `SMTP_*` settings point at a working mail server) |
| `DEMO_SEED` | `true` (loads the 12 cars on first start) |
| `DEMO_ADMIN_EMAIL` | the operator's private login, for example `op-xxxxxxxxxx@marque.test` |
| `DEMO_ADMIN_PASSWORD` | a long random password (15 characters or more) |

The public demo operator (`admin@marque.test`, a known password) is not created when the two `DEMO_ADMIN_*` values are set, and is demoted if it exists. Without the private login, the chaos lab and the audit are unavailable to anyone.

**Start it** (one API instance, so the chaos switch and the bots act on the instance that serves every bid):

```bash
docker compose -f deploy/compose.yml -f deploy/showcase.yml up -d --build --wait
```

The first build compiles Go on the ARM machine and takes several minutes. Check it:

```bash
curl -s https://api.<your-ip>.nip.io/readyz
# {"checks":{"elasticsearch":"ok","kafka":"ok","postgres":"ok","redis":"ok"},"status":"ok"}
```

Elasticsearch and Kafka need about 2 GB between them; with 12 GB you have room. Grafana, Prometheus, Jaeger and Mailpit are bound to 127.0.0.1 only (reach them over `ssh -L`).

## 2. The frontend on Vercel

1. Vercel, Add New Project, import the GitHub repository.
2. **Root Directory: `frontend`**. Framework preset: Vite (`vercel.json` already sets `npm run build`, the `dist` output folder, the single-page rewrite and the headers).
3. Deploy. The site already works on the demo engine.
4. Point it at the backend: Project Settings, Environment Variables, add `VITE_API_URL` = the API's origin (`https://api.<your-ip>.nip.io`, or your Render URL, no trailing slash), then redeploy. (A visitor can also try `?api=https://...` once; the choice is remembered.)
5. Put the Vercel URL in `CORS_ALLOWED_ORIGINS` on the VM and restart the API:

```bash
docker compose -f deploy/compose.yml -f deploy/showcase.yml up -d api-1
```

Open the Vercel URL: the pill at the bottom left should say **Live backend** with a green dot.

## Keeping the demo stocked (also on Render)

Seeded lots close on a timer, and a closed auction stays closed. With `DEMO_SEED=true` the API therefore seeds at start-up and again every `DEMO_REFRESH_EVERY` (default 10 minutes): any catalogue car that no longer has an open lot gets a fresh one (the closed auction and its bids stay). `DEMO_LOT_HOURS` makes new lots last at least that long, on top of their catalogue length (use `72` for a public demo; the catalogue alone is 20 minutes to 12 hours).

Hosting the API on Render, or anywhere else, needs only environment variables, no SQL:

```
DEMO_BOTS_ENABLED=true
DEMO_SEED=true
DEMO_LOT_HOURS=72
CORS_ALLOWED_ORIGINS=https://<your-frontend>.vercel.app
```

Deploy, and the closed lots are replaced within a minute. An operator can also press "Refresh demo cars" in the badge (or `POST /v1/demo/seed`) to do it on demand.

## What to know

- **Sign in as the operator** with your private login to use the chaos lab, the books-balance audit and the metrics. Everyone else can bid with their own accounts (register needs email activation, which goes to Mailpit unless you configure SMTP; the seeded `demo@marque.test` and `collector@marque.test` work with `marque-demo-password`).
- **Anyone can flip the demo bots** from the badge once signed in. That is the point of a demo, and it is why this is not a production setup. Set `DEMO_BOTS_ENABLED=false` to remove the bots.
- **The chaos switch needs the operator login**, so visitors cannot break your backend unless they have it.
- **Updating:** `git pull && docker compose -f deploy/compose.yml -f deploy/showcase.yml up -d --build --wait`.
- **Backups:** `deploy/backup.sh` runs nightly inside the stack. For a showcase you can also just re-seed.
- **A quiet machine is cheap, a loud one is not:** Oracle's free ARM allowance covers this stack. Nothing here needs a paid plan.
