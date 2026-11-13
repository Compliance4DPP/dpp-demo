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

1. Copy the template: `cp .env.example .env` (`.env` is gitignored; never commit the key). It is the project's only env file; `docker-compose.yaml` gives each container just the variables it reads.
2. In `.env`, replace `PASTE_KEY_HERE` with the RSA private key as PEM, line breaks included, inside the quotes.
3. Recreate the containers so the key is loaded: `docker compose up -d --force-recreate`

The issuer defaults to `did:web:validate.posedio.com`, key `2026-v3`. The key must be the private half of that entry in the issuer's `did.json`, or verification on the page fails. To use another issuer, set `VC_DID_HOST` and `VC_KEY_ID` in `.env`.

Any change to `.env` or to `policy/` needs the recreate command again; changes in `web/` only need a page reload.

### Publishing on Pontus-X

After a passing check, **Publish** sends the DPP and its compliance credential to the publishing stack, which publishes the DPP as a Pontus-X offering with a Gaia-X service offering of the provider. It needs the stack running (`docker compose --profile publish up --build`) and its settings in `.env`; see [publish/README.md](publish/README.md).

### Development

```bash
make test          # policy tests
make test_publish  # publish service tests (go vet + go test), see publish/README.md
make fmt           # format policy/ and web/ before committing; CI checks it
```

## Notes

- Demo only: anyone who can reach the page gets a credential signed by the issuer for any input that passes. The port is bound to localhost.
- The Gaia-X ComplianceCredential shape is a demo mapping, not a Gaia-X compliance claim.
- "also require technical data" is a demo profile, not part of IDTA 02099-1.
