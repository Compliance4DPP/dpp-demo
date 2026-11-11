# DPP metadata check against IDTA 02099-1 v1.0, sections 3.1 and 3.2:
# AAS structure, submodel identity, mandatory/optional attributes, types, semanticIds,
# granularity values and assetKind mapping, supplementalSemanticIds, contentSpecificationIds list,
# lastUpdate date-time (UTC).
package dpp

import rego.v1

cds := "https://admin-shell.io/idta/cds/"

# idShort -> expected valueType, mandatory (cardinality 1)
attributes := {
	"digitalProductPassportId": {"valueType": "xs:string", "mandatory": true},
	"uniqueProductIdentifier": {"valueType": "xs:string", "mandatory": true},
	"granularity": {"valueType": "xs:string", "mandatory": true},
	"dppSchemaVersion": {"valueType": "xs:string", "mandatory": true},
	"dppStatus": {"valueType": "xs:string", "mandatory": true},
	"lastUpdate": {"valueType": "xs:dateTime", "mandatory": true},
	"economicOperatorId": {"valueType": "xs:string", "mandatory": true},
	"facilityId": {"valueType": "xs:string", "mandatory": false},
}

granularity_values := {"Model", "Item", "Batch"}

shells := object.get(input, "assetAdministrationShells", [])

all_submodels := object.get(input, "submodels", [])

candidates := [s | some s in all_submodels; s.idShort == "DppMetadata"]

submodel := candidates[0]

els := object.get(submodel, "submodelElements", [])

ids := {x.idShort | some x in els; is_string(x.idShort)}

# first element per idShort; duplicates are reported in DUP-*
elements[id] := e if {
	some id in ids
	e := [x | some x in els; x.idShort == id][0]
}

result(check, status, message) := {"check": check, "status": status, "message": message}

bool_status(true) := "PASS"

bool_status(false) := "FAIL"

sem_ok(node, expected) := true if {
	some k in node.semanticId.keys
	k.value == expected
} else := false

# ---- 0. AAS structure ----
default roots_ok := false

roots_ok if {
	is_array(input.assetAdministrationShells)
	is_array(input.submodels)
}

results contains result("AAS-roots", bool_status(roots_ok), "'assetAdministrationShells' and 'submodels' are arrays")

shell_ok(s) := true if {
	is_string(s.id)
	s.id != ""
	is_object(s.assetInformation)
} else := false

results contains result("AAS-shell", bool_status(shell_ok(s)), sprintf("shell '%v' has id and assetInformation", [object.get(s, "idShort", "?")])) if some s in shells

submodel_ok(s) := true if {
	is_string(s.id)
	s.id != ""
} else := false

results contains result("AAS-submodel-id", bool_status(submodel_ok(s)), sprintf("submodel '%v' has an id", [object.get(s, "idShort", "?")])) if some s in all_submodels

submodel_ids := {s.id | some s in all_submodels; is_string(s.id)}

refs contains k.value if {
	some sh in shells
	some r in object.get(sh, "submodels", [])
	some k in r.keys
	k.type == "Submodel"
}

unresolved := refs - submodel_ids

results contains result("AAS-refs", "PASS", "all submodel references resolve") if {
	count(shells) > 0
	count(unresolved) == 0
}

results contains result("AAS-refs", "FAIL", sprintf("unresolved submodel references: %v", [unresolved])) if count(unresolved) > 0

results contains result("AAS-dpp-referenced", bool_status(submodel.id in refs), "DppMetadata is referenced by an AAS") if submodel

# ---- 1. DppMetadata submodel ----
results contains result("STRUCT-submodel", "FAIL", "No submodel with idShort 'DppMetadata'") if count(candidates) == 0

results contains result("STRUCT-submodel", "PASS", "DppMetadata submodel found") if count(candidates) == 1

results contains result("STRUCT-submodel", "FAIL", "More than one submodel with idShort 'DppMetadata'") if count(candidates) > 1

results contains result("SEM-submodel", bool_status(sem_ok(submodel, concat("", [cds, "dppMetadata/1"]))), "submodel semanticId is .../cds/dppMetadata/1") if submodel

results contains result(sprintf("DUP-%s", [id]), "FAIL", sprintf("idShort '%s' occurs more than once", [id])) if {
	some id in ids
	count([x | some x in els; x.idShort == id]) > 1
}

# ---- 2. mandatory / optional presence ----
presence_status(_, true) := "PASS"

presence_status(true, false) := "FAIL"

presence_status(false, false) := "SKIP"

cond_name(true) := "Mandatory"

cond_name(false) := "Optional"

presence_text(true) := "present"

presence_text(false) := "missing"

results contains result(sprintf("MAND-%s", [id]), status, msg) if {
	submodel
	some id, a in attributes
	present := object.get(elements, id, null) != null
	status := presence_status(a.mandatory, present)
	msg := sprintf("%s attribute '%s' %s", [cond_name(a.mandatory), id, presence_text(present)])
}

# a mandatory attribute must carry a non-blank string value
has_value(e) := true if {
	is_string(e.value)
	trim_space(e.value) != ""
} else := false

results contains result(sprintf("VAL-%s-nonempty", [id]), bool_status(has_value(e)), sprintf("mandatory attribute '%s' has a non-blank value", [id])) if {
	some id, a in attributes
	a.mandatory
	e := elements[id]
}

# ---- 3. type checks and semanticIds (only for present elements) ----
type_ok(e, a) := true if {
	e.modelType == "Property"
	e.valueType == a.valueType
} else := false

results contains result(sprintf("TYPE-%s", [id]), bool_status(type_ok(e, a)), msg) if {
	some id, a in attributes
	e := elements[id]
	msg := sprintf("modelType=%v valueType=%v, expected Property/%s", [object.get(e, "modelType", "none"), object.get(e, "valueType", "none"), a.valueType])
}

results contains result(sprintf("SEM-%s", [id]), bool_status(sem_ok(e, concat("", [cds, id, "/1"]))), sprintf("semanticId is .../cds/%s/1", [id])) if {
	some id, _ in attributes
	e := elements[id]
}

# supplementalSemanticIds required by the spec on granularity (AssetKind) and lastUpdate (updatedAt)
supplemental := {
	"granularity": "https://admin-shell.io/aas/3/2/AssetKind",
	"lastUpdate": "https://admin-shell.io/aas/3/2/AdministrativeInformation/updatedAt",
}

supp_ok(e, expected) := true if {
	some r in e.supplementalSemanticIds
	some k in r.keys
	k.value == expected
} else := false

results contains result(sprintf("SEM-%s-supplemental", [id]), bool_status(supp_ok(elements[id], expected)), sprintf("supplementalSemanticId is %s", [expected])) if {
	some id, expected in supplemental
	elements[id]
}

# ---- 4. granularity enumeration ----
results contains result("VAL-granularity", bool_status(object.get(elements.granularity, "value", null) in granularity_values), sprintf("granularity = %v, expected Model, Item or Batch", [object.get(elements.granularity, "value", null)])) if elements.granularity

# Table 2: granularity must map to the AAS assetKind of the shell that references the DPP submodel
asset_kind := {"Model": "Type", "Item": "Instance", "Batch": "Batch"}

dpp_shells contains sh if {
	some sh in shells
	some r in object.get(sh, "submodels", [])
	some k in r.keys
	k.value == submodel.id
}

kind_ok(sh, g) := true if {
	sh.assetInformation.assetKind == asset_kind[g]
} else := false

results contains result("VAL-granularity-assetKind", bool_status(kind_ok(sh, g)), sprintf("granularity %v maps to assetKind %v; shell '%v' has %v", [g, object.get(asset_kind, g, "none"), object.get(sh, "idShort", "?"), object.get(object.get(sh, "assetInformation", {}), "assetKind", "none")])) if {
	g := elements.granularity.value
	some sh in dpp_shells
}

# ---- 5. contentSpecificationIds (SML, optional, section 3.2) ----
csi := elements.contentSpecificationIds

csi_items := v if {
	v := csi.value
	is_array(v)
} else := []

csi_ok(e) := true if {
	e.modelType == "SubmodelElementList"
	e.typeValueListElement == "Property"
	e.valueTypeListElement == "xs:string"
	object.get(e, "orderRelevant", true) == false
} else := false

results contains result("OPT-contentSpecificationIds", "SKIP", "Optional attribute 'contentSpecificationIds' missing") if {
	submodel
	not csi
}

results contains result("OPT-contentSpecificationIds", "PASS", "Optional attribute 'contentSpecificationIds' present") if csi

results contains result("SML-contentSpecificationIds", bool_status(csi_ok(csi)), "SubmodelElementList of Property/xs:string, orderRelevant=false") if csi

results contains result("SEM-contentSpecificationIds", bool_status(sem_ok(csi, concat("", [cds, "contentSpecificationIds/1"]))), "semanticId is .../cds/contentSpecificationIds/1") if csi

results contains result("SML-contentSpecificationIds-count", bool_status(count(csi_items) >= 1), sprintf("%d item(s), at least 1 required", [count(csi_items)])) if csi

item_ok(it) := true if {
	it.modelType == "Property"
	it.valueType == "xs:string"
	is_string(it.value)
	trim_space(it.value) != ""
} else := false

results contains result(sprintf("SML-item-%d", [i]), bool_status(item_ok(it)), "item is Property/xs:string with a non-empty value") if some i, it in csi_items

results contains result(sprintf("SEM-item-%d", [i]), bool_status(sem_ok(it, concat("", [cds, "contentSpecificationId/1"]))), "item semanticId is .../cds/contentSpecificationId/1") if some i, it in csi_items

# ---- 6. date-time: well-formed, and timezone required (spec: lastUpdate is UTC-based) ----
dt_re := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`

has_tz(v) := regex.match(`(Z|[+-]\d{2}:\d{2})$`, v)

with_tz(v) := v if has_tz(v)

with_tz(v) := concat("", [v, "Z"]) if not has_tz(v)

# calendar/range validation is delegated to the RFC 3339 parser (tz appended when absent)
well_formed(v) := true if {
	regex.match(dt_re, v)
	time.parse_rfc3339_ns(with_tz(v))
} else := false

results contains result("DT-lastUpdate-wellformed", bool_status(well_formed(v)), sprintf("lastUpdate = %v", [v])) if {
	v := elements.lastUpdate.value
	is_string(v)
}

results contains result("DT-lastUpdate-timezone", bool_status(has_tz(v)), "timezone designator (Z or +hh:mm) is required") if {
	v := elements.lastUpdate.value
	is_string(v)
	well_formed(v)
}

# spec: "Timestamp UTC-based", so only Z or an offset of zero is accepted
results contains result("DT-lastUpdate-utc", bool_status(regex.match(`(Z|[+-]00:00)$`, v)), "UTC required (Z or +00:00)") if {
	v := elements.lastUpdate.value
	is_string(v)
	well_formed(v)
	has_tz(v)
}

# ---- report ----
make_report(rs) := {"allow": count(f) == 0, "failed": count(f), "results": sort(rs)} if {
	f := [r | some r in rs; r.status == "FAIL"]
}

# profile "IDTA only" (default) and profile "IDTA + technical data"
report := with_vc(make_report(results))

report_tech := with_vc(make_report(results | tech_results))

# ---- compliance credential (only when the check passes) ----
# Shape follows the Gaia-X ComplianceCredential (https://docs.gaia-x.eu/ontology/development/classes/ComplianceCredential/).
# Signed as VC-JWT when VC_KEY (RSA private key PEM, from the gitignored .env) is set; unsigned otherwise.
# Issuer and key id come from the environment (.env); defaults: validate.posedio.com, key 2026-v3 (PS256, expires 2027-05-08).
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

# ---- 7. technical data (demo only, not part of IDTA 02099-1) ----
# Port of check_technical_data in gaiax_demonstrator: a submodel whose idShort contains "technical"
# (case, spaces, _ and - ignored) with >= 3 elements that carry a value, referenced by an AAS.
# Only evaluated in report_tech; a missing technical submodel fails there, as in the Python checker.
tech_min_elements := 3

norm_name(s) := regex.replace(lower(s), `[^a-z0-9]`, "")

tech_candidates := [s |
	some s in all_submodels
	s.id != object.get(submodel, "id", null)
	is_string(s.idShort)
	contains(norm_name(s.idShort), "technical")
]

# element nodes sit at paths [i] / [i, "value", j] / ...; collections and lists are not counted themselves
tech_element(path) if {
	count(path) % 2 == 1
	every i, p in path {
		path_step_ok(i, p)
	}
}

path_step_ok(i, _) if i % 2 == 0

path_step_ok(i, "value") if i % 2 == 1

leaf_has_value(e) if {
	e.modelType == "MultiLanguageProperty"
	some t in e.value
	trim_space(t.text) != ""
}

leaf_has_value(e) if {
	e.modelType == "Range"
	some k in ["min", "max"]
	not object.get(e, k, "") in {"", null}
}

leaf_has_value(e) if {
	not e.modelType in {"MultiLanguageProperty", "Range"}
	v := e.value
	v != null
	not v in {[], {}}
	not blank(v)
}

blank(v) if {
	is_string(v)
	trim_space(v) == ""
}

tech_counts[i] := [count(w), count(wo)] if {
	some i, s in tech_candidates
	leaves := [n |
		walk(object.get(s, "submodelElements", []), [path, n])
		tech_element(path)
		is_object(n)
		not n.modelType in {"SubmodelElementCollection", "SubmodelElementList"}
	]
	w := [n | some n in leaves; leaf_has_value(n)]
	wo := [n | some n in leaves; not leaf_has_value(n)]
}

# best match: most elements with a value, first of equals
tech_best := i if {
	top := max({c[0] | some c in tech_counts})
	i := min({j | some j, c in tech_counts; c[0] == top})
}

tech := tech_candidates[tech_best]

tech_results contains result("TECH-submodel", "FAIL", "no submodel with 'technical' in its idShort") if {
	submodel
	count(tech_candidates) == 0
}

tech_results contains result("TECH-submodel", "PASS", sprintf("technical-data submodel '%s' found (%d candidate(s))", [tech.idShort, count(tech_candidates)])) if tech

tech_results contains result("TECH-elements", bool_status(is_array(tech.submodelElements)), "submodelElements is missing or not an array") if {
	tech
	not is_array(tech.submodelElements)
}

tech_results contains result("TECH-elements", bool_status(c[0] >= tech_min_elements), sprintf("%d element(s) with a value, %d without; at least %d required", [c[0], c[1], tech_min_elements])) if {
	is_array(tech.submodelElements)
	c := tech_counts[tech_best]
}

tech_results contains result("TECH-ref", bool_status(tech.id in refs), sprintf("'%s' is referenced by an AAS", [tech.idShort])) if tech
