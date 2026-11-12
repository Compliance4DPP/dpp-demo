# dpp-demo

Checks a Digital Product Passport (AAS JSON) against [IDTA 02099-1 v1.0, §3.1](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2026/05/IDTA-02099-1_Submodel-Digital-Product-Passport-Part-1.pdf) with OPA. A file that passes gets a signed verifiable credential, which the page verifies in the browser.

## Setup

You need Docker (with Compose). Node is only needed for `make fmt`.

```bash
git clone https://github.com/Compliance4DPP/dpp-demo.git
cd dpp-demo
docker compose up        # then open http://localhost:8082
```

That is enough to run checks; credentials are returned unsigned. Click a sample, or paste your own AAS JSON. The "How it works" page explains the flow.

### Signed credentials

1. Copy the template: `cp .env.example .env` (`.env` is gitignored; never commit the key).
2. In `.env`, replace `PASTE_KEY_HERE` with the RSA private key as PEM, line breaks included, inside the quotes.
3. Recreate the containers so the key is loaded: `docker compose up -d --force-recreate`

The issuer defaults to `did:web:validate.posedio.com`, key `2026-v3`. The key must be the private half of that entry in the issuer's `did.json`, or verification on the page fails. To use another issuer, set `VC_DID_HOST` and `VC_KEY_ID` in `.env`.

Any change to `.env` or to `policy/` needs the recreate command again; changes in `web/` only need a page reload.

### Development

```bash
make test        # policy tests and signature check tests (needs Node 22+)
make fmt         # format policy/ and web/ before committing; CI checks it
```

## Notes

- Demo only: anyone who can reach the page gets a credential signed by the issuer for any input that passes. The port is bound to localhost.
- The Gaia-X ComplianceCredential shape is a demo mapping, not a Gaia-X compliance claim.
- "also require technical data" is a demo profile, not part of IDTA 02099-1.
