# dpp-demo

OPA demonstrator for a Digital Product Passport (AAS) check against [IDTA 02099-1 v1.0, §3.1](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2026/05/IDTA-02099-1_Submodel-Digital-Product-Passport-Part-1.pdf).
Backend: OPA. Frontend: plain HTML + JS calling OPA's Data API through nginx.
Optional profile "also require technical data" (`data.dpp.report_tech`, demo only, not part of IDTA 02099-1) additionally checks for a technical-data submodel; example: `samples/valid_tech_desc_dpp.json`.

```bash
docker compose up   # http://localhost:8082
make test           # opa test
make fmt            # opa fmt for policy/, Prettier for web/ (needs Node); CI fails on unformatted files
```

## Layout

| Path | Content |
|---|---|
| `policy/dpp.rego` | The rules, in eight numbered groups, and the report |
| `policy/vc.rego` | The compliance credential: build, sign, error handling |
| `policy/dpp_test.rego` | `opa test` cases for both |
| `web/` | Check page, How it works page, one script, one stylesheet |
| `nginx.conf` | Serves `web/` and `samples/`; proxies the two report paths to OPA and `/did.json` to the issuer |
| `.env.example` | Template for the signing key and issuer settings |

## Report

`POST /v1/data/dpp/report` (or `report_tech`) with `{"input": <AAS JSON>}` returns:

| Field | Content |
|---|---|
| `allow` | `true` when no check failed |
| `failed` | Number of failed checks |
| `groups` | The rule group names in display order |
| `results` | One entry per check: `group`, `check` (id), `status` (`PASS`, `FAIL`, `SKIP`), `message` |
| `vc`, `vc_jwt`, `vc_error` | Only when `allow` is true; see Compliance credential |

The group of a check is set in the policy (the number in each `result(...)` call indexes `groups`), so the page does not guess it from the check id.

## Page

- **Check** (`web/index.html`): input on the left, result on the right (one column below 1100px). The result is the verdict, the credential card, pass/fail/skip counters, and the checks grouped by rule group; groups with a failure open on their own, failures first.
- **How it works** (`web/how.html`): the request flow as a diagram with one column per party (Browser, nginx, OPA, Issuer) and arrows for requests and answers, what the credential contains, the rule groups and the verdict legend.

## Compliance credential

When a check passes, OPA also issues a verifiable credential: a signed statement that this file passed, which others can verify against the issuer's public key without running the check again. A file with any FAIL gets none.

The report then also carries:

| Field | Content |
|---|---|
| `vc` | The credential as JSON, shaped like the Gaia-X [ComplianceCredential](https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/): issuer, 90 days validity, id of the checked DppMetadata submodel, SHA-256 digest of the input, list of passed checks |
| `vc_jwt` | The same credential signed as a VC-JWT (PS256). Only with a usable signing key |
| `vc_error` | Why it was not signed, when `VC_KEY` is set but unusable. The check result is still returned |

The page shows the credential in a card under the verdict and verifies the JWT right away in the browser (WebCrypto) against the issuer's DID document, which nginx proxies at `/did.json`. Editing the JWT and clicking "Verify signature", a wrong key, or a key revoked in the DID document makes verification fail; an unusable key turns the card red.

### Configuration

Copy `.env.example` to `.env` (gitignored) and paste the private key. Never put the key in the repo.

| Variable | Default | Meaning |
|---|---|---|
| `VC_KEY` | none (credential stays unsigned) | RSA private key as PEM, line breaks included |
| `VC_DID_HOST` | `validate.posedio.com` | Issuer is `did:web:<host>`; DID document is read from `https://<host>/.well-known/did.json` |
| `VC_KEY_ID` | `2026-v3` | Verification method in that DID document; must belong to `VC_KEY` |

OPA and nginx read these only at start, and OPA does not watch the policy either, so recreate after any change:

```bash
docker compose up -d --force-recreate
```

### Limits

- Demo only: anyone who can reach the page gets a credential signed by the issuer for any input that passes. The port is bound to localhost.
- Only host-only `did:web` issuers (no path component) and RSA keys (PS256).
- The digest is over OPA's canonical JSON of the input (sorted keys), not the bytes of the uploaded file.
- `gx:ComplianceCredential` and the check ids in `gx:validatedCriteria` are a demo mapping of the Gaia-X shape, not a Gaia-X compliance claim.
- `policy/dpp_test.rego` contains a throwaway private key used only to test signing.
