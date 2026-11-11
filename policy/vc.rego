# Compliance credential for a passing DPP check; see with_vc, used by report and report_tech in dpp.rego.
package dpp

import rego.v1

# Shape follows the Gaia-X ComplianceCredential (https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/).
# The default key 2026-v3 expires 2027-05-08.
# ponytail: host-only did:web (no path component); extend if an issuer DID with a path is ever needed.
env := opa.runtime().env

vc_issuer := concat("", ["did:web:", object.get(env, "VC_DID_HOST", "validate.posedio.com")])

vc_kid := concat("#", [vc_issuer, object.get(env, "VC_KEY_ID", "2026-v3")])

vc_valid_ns := ((90 * 24) * 3600) * 1000000000

vc_key := env.VC_KEY

vc_jwk := crypto.x509.parse_rsa_private_key(vc_key)

vc_header := {"alg": "PS256", "typ": "vc+jwt", "cty": "vc", "iss": vc_issuer, "kid": vc_kid}

with_vc(r) := object.union(r, vc_fields(credential(r))) if r.allow

with_vc(r) := r if not r.allow

# undefined when the key is missing or unusable (builtin errors are undefined, not fatal)
sign(vc) := io.jwt.encode_sign(vc_header, vc, vc_jwk)

vc_fields(vc) := {"vc": vc, "vc_jwt": jwt} if jwt := sign(vc)

vc_fields(vc) := {"vc": vc} if not vc_key

# a broken key must not take the check result down with it
vc_fields(vc) := {"vc": vc, "vc_error": "VC_KEY is set but is not a usable RSA private key; the credential was not signed"} if {
	vc_key
	not sign(vc)
}

credential(r) := {
	"@context": ["https://www.w3.org/ns/credentials/v2", "https://w3id.org/gaia-x/2.5.0#"],
	"type": ["VerifiableCredential", "gx:ComplianceCredential"],
	"id": id,
	"issuer": vc_issuer,
	"validFrom": time.format(now),
	"validUntil": time.format(now + vc_valid_ns),
	"credentialSubject": {
		"id": concat("", [id, "#cs"]),
		"gx:engineVersion": object.get(opa.runtime(), "version", "unknown"),
		"gx:rulesVersion": "IDTA-02099-1 v1.0",
		"gx:compliantCredentials": [{
			"id": submodel.id,
			"gx:credentialType": "DppMetadata",
			# digest of OPA's canonical JSON of the input (sorted keys), not of the uploaded file bytes
			"digestSRI": concat("", ["sha256-", crypto.sha256(json.marshal(input))]),
		}],
		"gx:validatedCriteria": [x.check | some x in r.results; x.status == "PASS"],
	},
} if {
	now := time.now_ns()
	id := concat("", ["urn:uuid:", uuid.rfc4122("vc")])
}
