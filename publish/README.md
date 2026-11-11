# publish

Slim gRPC service that publishes a DPP offering on Pontus-X:

1. loads the DPP (a file in `DPP_DIR` or the request body),
2. renders the template's credentials and signs each with the
   [gaia-x-signer](https://git.dumss.net/theia/gaia-x-signer) (`SignVC`),
3. renders the template's offering and publishes it through the
   [Ecosystems Gateway](https://github.com/PTW-TUD/Ecosystems-Gateway/tree/nautilus_v2) (`CreateOffering`).

The service hosts nothing: the offering's file URL and its credential URLs are fixed in the
template, and the signed credentials are returned to the caller to host. All templates are
rendered and checked before anything is signed or published.

## API

`dppdemo.publish.v1.Publisher/Publish` on port 50051 ([proto](proto/publish/v1/publish.proto)).
gRPC reflection and the health service are enabled.

```bash
grpcurl -plaintext -d '{"dpp_name": "valid_dpp", "template": "default"}' \
  localhost:50051 dppdemo.publish.v1.Publisher/Publish
```

`dpp_json` (bytes, base64 in JSON) can be sent instead of `dpp_name`. The response holds the
offering DID(s) and each signed credential as compact JWS. Publishing again creates a new offering.

## Templates

`TEMPLATE_DIR/<name>/` holds Go [text/templates](https://pkg.go.dev/text/template):

| File                       | Rendered to                                                                     |
| -------------------------- | ------------------------------------------------------------------------------- |
| `offering.json`            | a gateway `PontusxOffering` ([spp v3](proto/ecosystemsgateway/v3/spp_v3.proto)) |
| `credentials/<name>.json`  | a JSON-LD VC to sign, returned as `credentials[<name>]` (zero or more)          |

| In templates                 |                                                                    |
| ---------------------------- | ------------------------------------------------------------------ |
| `.DPP`                       | the parsed AAS JSON                                                |
| `.ID`, `.Issuer`, `.Now`     | DPP id (`digitalProductPassportId`, else the first shell's id), `ISSUER_DID`, request time (RFC 3339 UTC) |
| `uuid`                       | a random `urn:uuid:…`, e.g. for credential ids                     |
| `prop .DPP "idShort"`        | value of the first AAS Property with that idShort                  |
| `env "TEMPLATE_X"`           | an environment variable; only `TEMPLATE_*` ones are readable       |
| `default "x" v`, `json v`    | fallback for empty strings; encode as a JSON literal (use for every value) |

[`templates/default`](templates/default) publishes a free download with one `gx:ServiceOffering`
credential. Its file URL (`https://example.org/dpp/endmill-0001.json`) and
`serviceOfferingCredential` URL are placeholders to replace. The credential is a starting point:
align it with the shapes the Gaia-X compliance service you target checks.

## Configuration

Environment variables; see [`.env.example`](.env.example). Required: `SIGNER_ADDR`, `GATEWAY_ADDR`.

## Run

```bash
cp publish/.env.example publish/.env                      # signer auth, template values
cp publish/gateway.env.example publish/gateway.env        # wallet key, DDO store
cp publish/signer/config.example.yaml publish/signer/config.yaml  # DID; signing key as publish/signer/key.pem
docker compose --profile publish up --build
```

The signer runs from its published image; the gateway is built from its repository.

## Development

```bash
make test        # go vet + go test, in docker
make proto       # regenerate gen/ from proto/
make proto-sync  # refresh the vendored gateway proto from nautilus_v2, then regenerate
make tidy
```

`proto/signer/signer.proto` is a minimal, wire-compatible copy of the signer's `SignVC`;
`proto/ecosystemsgateway/v3/spp_v3.proto` is the gateway's proto without its HTTP/OpenAPI options.
