# dpp-demo

OPA demonstrator for a Digital Product Passport (AAS) check against [IDTA 02099-1 v1.0, §3.1](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2026/05/IDTA-02099-1_Submodel-Digital-Product-Passport-Part-1.pdf).
Backend: OPA (`policy/dpp.rego`). Frontend: plain HTML + JS calling OPA's Data API through nginx.
Optional profile "also require technical data" (`data.dpp.report_tech`, demo only, not part of IDTA 02099-1) additionally checks for a technical-data submodel; example: `samples/valid_tech_desc_dpp.json`.

When a check passes, the report carries `vc`: a credential shaped like the Gaia-X [ComplianceCredential](https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/) (90 days validity, digest of the checked input).
With a signing key it also carries `vc_jwt`, the same credential signed (PS256) as `did:web:validate.posedio.com#2026-v3` (change with `VC_DID_HOST` and `VC_KEY_ID`). Paste the PEM into `VC_KEY="..."` in a gitignored `.env` (template: `.env.example`), never in the repo.

```bash
docker compose up   # http://localhost:8082
make test           # opa test
```
