# dpp-demo

OPA demonstrator for a Digital Product Passport (AAS) check against [IDTA 02099-1 v1.0, §3.1](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2026/05/IDTA-02099-1_Submodel-Digital-Product-Passport-Part-1.pdf).
Backend: OPA (`policy/dpp.rego`). Frontend: plain HTML + JS calling OPA's Data API through nginx.
Optional profile "also require technical data" (`data.dpp.report_tech`, demo only, not part of IDTA 02099-1) additionally checks for a technical-data submodel; example: `samples/valid_tech_desc_dpp.json`.

```bash
docker compose up   # http://localhost:8082
make test           # opa test
```

## Compliance credential

When a check passes, OPA also issues a verifiable credential: a signed statement that this file passed, which others can verify against the issuer's public key without running the check again. A file with any FAIL gets none.

The report (`POST /v1/data/dpp/report`) then carries:

| Field | Content |
|---|---|
| `vc` | The credential as JSON, shaped like the Gaia-X [ComplianceCredential](https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/): issuer, 90 days validity, id of the checked DppMetadata submodel, SHA-256 digest of the input, list of passed checks |
| `vc_jwt` | The same credential signed as a VC-JWT (PS256). Only with a usable signing key |
| `vc_error` | Why it was not signed, when `VC_KEY` is set but unusable. The check result is still returned |

The page shows the credential under the verdict and verifies the JWT in the browser (WebCrypto) against the issuer's DID document, which nginx proxies at `/did.json`. Editing the JWT, a wrong key, or a key revoked in the DID document makes verification fail.

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
