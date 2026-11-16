# Compliance credential for a passing DPP check; see with_vc, used by report and report_tech in dpp.rego.
# Flow: with_vc(report) -> credential(report) builds the unsigned VC -> vc_fields signs it as a JWT
# with VC_KEY (if set) -> the VC (and JWT, or an error note) are merged into the report.
# Configured via environment variables: VC_KEY, VC_DID_HOST, VC_KEY_ID, VC_KEY_EXPIRES.
package dpp

import rego.v1

# Shape follows the Gaia-X ComplianceCredential (https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/).
# ponytail: host-only did:web (no path component); extend if an issuer DID with a path is ever needed.
env := opa.runtime().env

# issuer DID, e.g. did:web:validate.posedio.com
vc_issuer := concat("", ["did:web:", object.get(env, "VC_DID_HOST", "validate.posedio.com")])

# key id in the issuer's DID document, e.g. did:web:validate.posedio.com#2026-v3; verifiers use it to find the public key
vc_kid := concat("#", [vc_issuer, object.get(env, "VC_KEY_ID", "2026-v3")])

# credentials are valid for 90 days (in nanoseconds, the unit of time.now_ns)
vc_valid_ns := ((90 * 24) * 3600) * 1000000000

# a credential must not claim to be valid longer than its signing key; the default is the expiry of key 2026-v3.
# An unreadable VC_KEY_EXPIRES falls back to that default, so a typo cannot take the check result down.
vc_key_expires_ns := ns if {
	ns := time.parse_rfc3339_ns(env.VC_KEY_EXPIRES)
} else := time.parse_rfc3339_ns("2027-05-08T23:59:59Z")

# RSA private key (PEM); undefined when VC_KEY is not set
vc_key := env.VC_KEY

# the PEM converted to a JWK, the key format io.jwt.encode_sign expects
vc_jwk := crypto.x509.parse_rsa_private_key(vc_key)

# JOSE header of the signed credential (VC-JOSE-COSE: typ vc+jwt, RSA-PSS signature)
vc_header := {"alg": "PS256", "typ": "vc+jwt", "cty": "vc", "iss": vc_issuer, "kid": vc_kid}

# passing report: add the credential fields; failing report: returned unchanged
with_vc(r) := object.union(r, vc_fields(credential(r))) if r.allow

with_vc(r) := r if not r.allow

# undefined when the key is missing or unusable (builtin errors are undefined, not fatal)
sign(vc) := io.jwt.encode_sign(vc_header, vc, vc_jwk)

# exactly one of the three vc_fields rules applies:
# signed -> vc + vc_jwt; no key configured -> unsigned vc only; key set but unusable -> vc + vc_error
vc_fields(vc) := {"vc": vc, "vc_jwt": jwt} if jwt := sign(vc)

vc_fields(vc) := {"vc": vc} if not vc_key

# a broken key must not take the check result down with it
vc_fields(vc) := {"vc": vc, "vc_error": "VC_KEY is set but is not a usable RSA private key; the credential was not signed"} if {
	vc_key
	not sign(vc)
}

# the unsigned credential: who issued it, how long it is valid, and what it vouches for
# (the DppMetadata submodel, a digest of the checked input, and the ids of all checks that passed);
# its id is a random UUID, fresh on every evaluation
credential(r) := {
	"@context": ["https://www.w3.org/ns/credentials/v2", "https://w3id.org/gaia-x/2.5.0#"],
	"type": ["VerifiableCredential", "gx:ComplianceCredential"],
	"id": id,
	"issuer": vc_issuer,
	"validFrom": time.format(now),
	# 90 days, but never past the signing key's expiry
	"validUntil": time.format(min([now + vc_valid_ns, vc_key_expires_ns])),
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
