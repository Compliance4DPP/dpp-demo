# dpp-demo

OPA demonstrator for a Digital Product Passport (AAS) check against [IDTA 02099-1 v1.0, §3.1](https://industrialdigitaltwin.org/en/wp-content/uploads/sites/2/2026/05/IDTA-02099-1_Submodel-Digital-Product-Passport-Part-1.pdf).
Backend: OPA (`policy/dpp.rego`). Frontend: plain HTML + JS calling OPA's Data API through nginx.
Optional profile "also require technical data" (`data.dpp.report_tech`, demo only, not part of IDTA 02099-1) additionally checks for a technical-data submodel; example: `samples/valid_tech_desc_dpp.json`.

```bash
docker compose up   # http://localhost:8082
make test           # opa test
```
