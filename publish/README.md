# publish

Slim HTTP/JSON service (`dataswitch` in compose) that publishes a checked DPP on Pontus-X:

1. loads the DPP (request body or a file in `DPP_DIR`) and its compliance credential from the
   web demonstrator's check,
2. reads the provider's Gaia-X participant credential (`PROVIDER_CREDENTIAL_URL`),
3. renders the template's credentials (a `gx:DigitalServiceOffering`) and signs each with the
   [gaia-x-signer](https://git.dumss.net/theia/gaia-x-signer) (`POST /sdbs/v1/signVC`),
4. combines them with the participant's credentials into one presentation and has it signed
   (`POST /sdbs/v1/signVP`): the input of the Gaia-X compliance service,
5. renders the asset description (Markdown, with the compliance credential and the service
   offering as code blocks) and the offering, and publishes it through the hosted
   [Ecosystems Gateway](https://github.com/PTW-TUD/Ecosystems-Gateway/tree/nautilus_v2)
   (`POST <GATEWAY_URL>CreateOffering`, bearer token; API docs at `/docs` of its host).

The service hosts nothing: the offering's file URL and credential URLs come from the template,
and the signed presentation is returned to the caller to host at the URL the offering links.
All templates are rendered and checked before anything is signed or published.

The web demonstrator's **Publish** button (shown after a passing check) calls this service
through nginx (`POST /publish` on the page's origin).

## API

`POST /publish` on port 8080 (`127.0.0.1:8083` with compose, and `/publish` of the web page),
JSON in and out; `GET /healthz`.

```bash
curl -s localhost:8083/publish -H 'Content-Type: application/json' \
  -d '{"template": "default", "dpp_name": "valid_dpp", "compliance_credential": "eyJ…"}'
```

| Request field           | |
| ----------------------- | --- |
| `template`              | name of a template directory in `TEMPLATE_DIR`, e.g. `default` |
| `dpp`                   | the DPP itself, as AAS JSON object; or |
| `dpp_name`              | name of a DPP file in `DPP_DIR` without `.json`, e.g. `valid_dpp` |
| `compliance_credential` | the DPP's compliance credential from the check: its JWT (`vc_jwt`) as string, or the credential (`vc`) as object; optional |

| Response field     | |
| ------------------ | --- |
| `offering_ids`     | the published offering(s), e.g. `did:ope:…` |
| `credentials`      | each signed template credential as compact JWS, by name |
| `presentation`     | the signed credentials and the participant's, as one presentation (`vp+jwt`) |
| `presentation_url` | where the offering links it (`additionalInformation.gaiaX.serviceOfferingCredential`); the same for every offering, so the presentation is hosted there once |

Errors are `{"error": "…"}` with 400 (bad request), 404 (unknown template or DPP), 422 (template
fails, e.g. a required setting is missing), 502 (participant credential, signer or gateway failed,
with their status and message) or 504 (`PUBLISH_TIMEOUT` ran out). Publishing again creates a new
offering. The compliance credential's signature is not checked here; the description shows it,
and anyone can verify it against its issuer's `did.json`.

## Templates

`TEMPLATE_DIR/<name>/` holds Go [text/templates](https://pkg.go.dev/text/template):

| File                       | Rendered to                                                                     |
| -------------------------- | ------------------------------------------------------------------------------- |
| `credentials/<name>.json`  | a JSON-LD VC to sign, returned as `credentials[<name>]` (zero or more)          |
| `description.md`           | optional: Markdown, the asset description, as `.Description` in `offering.json` |
| `offering.json`            | a gateway `PontusxOffering`, as in its HTTP/JSON API (see its `/docs`)          |

The credentials are rendered first; `description.md` and `offering.json` then also see them,
signed, in `.Credentials`.

| In templates                     |                                                                    |
| -------------------------------- | ------------------------------------------------------------------ |
| `.DPP`                           | the parsed AAS JSON                                                |
| `.ID`, `.Issuer`, `.Now`         | DPP id (`digitalProductPassportId`, else the first shell's id), `ISSUER_DID`, request time (RFC 3339 UTC) |
| `.Provider`                      | the participant: `.ID` (its `gx:LegalPerson` subject, for `gx:providedBy`), `.Name`, `.Locality`, `.CountryCode`, `.CredentialURL` |
| `.Compliance`                    | the request's compliance credential (or nil): `.ID`, `.JSON` (indented), `.JWT`, `.Doc` |
| `.Credentials`, `.Description`   | the signed template credentials by name (as `.Compliance`); the rendered `description.md` |
| `uuid`                           | a random `urn:uuid:…`, e.g. for credential ids                     |
| `prop .DPP "idShort"`            | value of the first AAS Property with that idShort                  |
| `env "TEMPLATE_X"`               | an environment variable; only `TEMPLATE_*` ones are readable       |
| `default "x" v`, `required "what" v` | fallback for empty strings; fail with "what is required" if empty |
| `json v`, `md v`, `slug v`       | a JSON literal (use for every JSON value); a single Markdown line with `\|*_` etc. escaped; an id as URL path part (`example-com-dpp-x1`) |

### The default template

[`templates/default`](templates/default) publishes the free download of a cutting tool's DPP:

- **Service offering** (`credentials/service-offering.json`): a `gx:DigitalServiceOffering` of the
  [Gaia-X ontology](https://w3id.org/gaia-x/development) (development), provided by the
  participant (`gx:providedBy`), with terms and conditions (by default deltaDAO's Pontus-X
  portal terms, pinned to a commit with their SHA-256; own ones with `TEMPLATE_TERMS_URL` and
  `TEMPLATE_TERMS_SHA256`), an ODRL usage policy, data export and GDPR, and what the
  Gaia-X labelling criteria for digital services ask for: version, provider contact, data
  protection measures and data portability (`TEMPLATE_CONTACT_URL`, `TEMPLATE_CONTACT_EMAIL`,
  `TEMPLATE_DATA_PROTECTION_URL`, `TEMPLATE_DATA_PORTABILITY_URL`, `TEMPLATE_PRICING_URL`,
  `TEMPLATE_SERVICE_VERSION`; the defaults are `example.org` placeholders for the demo). The
  presentation it is part of passed the Gaia-X lab compliance service (development, rules
  CD4.0.0, label level SC) with PTW TU Darmstadt's participant credential. It is the same for
  every offering: one presentation of it is hosted once at `TEMPLATE_SERVICE_OFFERING_URL`
  (default `https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/dpp-service-offering.jwt`),
  which all offerings link; its id is that URL without `.jwt`. Rendered with the participant credential of PTW TU Darmstadt, it conforms to
  the Gaia-X development SHACL shapes once that participant credential's
  `gx:registrationNumber` points to the registration number's subject (`…/registrationNumber#cs`)
  rather than its credential (`…/registrationNumber`).
- **Description** (`description.md`): banner, the tool at a glance (product, manufacturer,
  technical data and DPP metadata, as far as the DPP has them), its submodels, DPP and AAS with a
  figure, the compliance credential (JSON and JWT) and the service offering as code blocks, the
  provider, COMPLIANCE4DPP and the EU funding notice. Images are loaded from
  `TEMPLATE_IMAGE_BASE_URL` (default `https://credentials.ptw.maschinenbau.tu-darmstadt.de/static/`); upload
  `templates/default/images/` there (the folder is not in git). The DPP/AAS figure is from the
  [AAS specification](https://github.com/admin-shell-io/aas-specs-metamodel) (IDTA, CC BY 4.0).
- **Offering** (`offering.json`): dataset, free access service with the file at
  `TEMPLATE_DPP_FILE_URL`, `providedBy` the participant credential. For the demonstrator, every
  offering delivers the same sample DPP by default,
  `https://credentials.ptw.maschinenbau.tu-darmstadt.de/static/tool_dpp.json` (upload `samples/tool_dpp.json` there),
  a fictional PTW end mill. The publish service uploads nothing: the file and the
  presentation (`presentation_url`) are hosted by the provider.

## Configuration

Everything is set in the root [`.env`](../.env.example), publishing section; `docker-compose.yaml`
passes each variable to the container that reads it.

| Variable                  | |
| ------------------------- | --- |
| `GATEWAY_URL`             | base URL of the hosted gateway's API, e.g. `https://tec-ecosystemgateway.ptw.maschinenbau.tu-darmstadt.de/grpc/` (required) |
| `GATEWAY_TOKEN`           | bearer token for it |
| `PROVIDER_CREDENTIAL_URL` | the provider's Gaia-X participant credential, e.g. `https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/participant.jwt` |
| `SIGNER_URL`              | the signer's HTTP API (required; `http://signer:8091` in compose) |
| `SIGNER_AUTHORIZATION`    | `Authorization` header for the signer; must match a `basicAuths` entry of its config |
| `ISSUER_DID`              | the credentials' issuer, the provider's DID; also passed to the signer as `GXCOMPLIANCE_DID`, overriding `gxCompliance.DID` of its config, so both cannot drift apart |
| `ISSUER_KEY_ID`           | the key in that DID's `did.json` whose private key is `publish/signer/key.pem`, e.g. `X509-JWK2020`; passed to the signer as `GXCOMPLIANCE_VERIFICATIONMETHOD` (`<ISSUER_DID>#<ISSUER_KEY_ID>`). Its algorithm must match the config's `signatureAlgorithm` (RS256 for an RSA key) |
| `TEMPLATE_*`              | values for the templates (see `.env.example`); a new one needs a line in `docker-compose.yaml` |
| `PUBLISH_TIMEOUT`, `MAX_REQUEST_BYTES`, `HTTP_ADDR` | default `10m`, 16 MiB, `:8080` |

## Run

```bash
cp .env.example .env   # if not done yet; set GATEWAY_TOKEN, ISSUER_DID and the terms
cp publish/signer/config.example.yaml publish/signer/config.yaml  # signing key as publish/signer/key.pem
docker compose --profile publish up --build
```

Then check a DPP on the web page and press **Publish**. The signer runs from its published image
with its HTTP gateway on port 8091 (only in the compose network); its gRPC port listens inside
its container only. Anyone who can reach the page can publish, at the cost of the gateway's
wallet: the page is bound to localhost.

## Development

```bash
make test_publish   # from the repo root: go vet + go test, in docker
```

The service uses only the Go standard library. Tests run against fake signer, gateway and
participant credential servers.
