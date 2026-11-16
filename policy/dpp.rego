# METADATA
# title: DPP metadata check
# description: >-
#   Checks an AAS environment against IDTA 02099-1 v1.0, sections 3.1 and 3.2:
#   AAS structure, submodel identity, mandatory/optional attributes, types, semanticIds,
#   granularity values and assetKind mapping, supplementalSemanticIds, contentSpecificationIds list,
#   lastUpdate date-time (UTC). Passing reports carry a Gaia-X compliance credential (vc.rego).
package dpp

import rego.v1

# prefix of the IDTA concept-description IRIs used as semanticIds, e.g. .../cds/dppStatus/1
cds := "https://admin-shell.io/idta/cds/"

# the DppMetadata attributes from the spec: expected valueType and whether a missing one fails (mandatory)
# or is only reported as SKIP (optional)
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

# ---- shared lookups used by the checks below ----
# object.get with a default keeps a missing key from making everything downstream undefined
shells := object.get(input, "assetAdministrationShells", [])

all_submodels := object.get(input, "submodels", [])

# all submodels named DppMetadata; more than one is a FAIL (STRUCT-submodel)
candidates := [s | some s in all_submodels; s.idShort == "DppMetadata"]

# the DPP submodel under test; undefined when there is none, which skips all checks that need it
submodel := candidates[0]

# its elements, and the set of their idShorts (non-string idShorts are ignored)
els := object.get(submodel, "submodelElements", [])

ids := {x.idShort | some x in els; is_string(x.idShort)}

# first element per idShort; duplicates are reported in DUP-*
elements[id] := e if {
	some id in ids
	e := [x | some x in els; x.idShort == id][0]
}

# rule groups, in display order; the number in each result(...) call is an index into this list
groups := [
	"AAS structure",
	"DppMetadata submodel",
	"Presence",
	"Types and semanticIds",
	"Granularity",
	"contentSpecificationIds",
	"lastUpdate",
	"Technical data",
]

# one row of the report: group name, check id (e.g. "TYPE-granularity"), PASS/FAIL/SKIP, human-readable message
result(g, check, status, message) := {"group": groups[g], "check": check, "status": status, "message": message}

# turns a boolean check into a status string
bool_status(true) := "PASS"

bool_status(false) := "FAIL"

# true if any key of node.semanticId equals `expected`; false (not undefined) otherwise
sem_ok(node, expected) if {
	some k in node.semanticId.keys
	k.value == expected
} else := false

# ---- 0. AAS structure ----
default roots_ok := false

roots_ok if {
	is_array(input.assetAdministrationShells)
	is_array(input.submodels)
}

# no `if`: this check is always reported
results contains result(0, "AAS-roots", bool_status(roots_ok), "'assetAdministrationShells' and 'submodels' are arrays")

# one AAS-shell / AAS-submodel-id result per shell / submodel in the input
shell_ok(s) if {
	is_string(s.id)
	s.id != ""
	is_object(s.assetInformation)
} else := false

results contains result(0, "AAS-shell", bool_status(shell_ok(s)), sprintf("shell '%v' has id and assetInformation", [object.get(s, "idShort", "?")])) if some s in shells

submodel_ok(s) if {
	is_string(s.id)
	s.id != ""
} else := false

results contains result(0, "AAS-submodel-id", bool_status(submodel_ok(s)), sprintf("submodel '%v' has an id", [object.get(s, "idShort", "?")])) if some s in all_submodels

submodel_ids := {s.id | some s in all_submodels; is_string(s.id)}

# ids of all submodels that some shell points to (its "submodels" references)
refs contains k.value if {
	some sh in shells
	some r in object.get(sh, "submodels", [])
	some k in r.keys
	k.type == "Submodel"
}

# references to submodels that are not in the file (set difference)
unresolved := refs - submodel_ids

# PASS and FAIL are separate rules with opposite conditions, so exactly one of them fires
results contains result(0, "AAS-refs", "PASS", "all submodel references resolve") if {
	count(shells) > 0
	count(unresolved) == 0
}

results contains result(0, "AAS-refs", "FAIL", sprintf("unresolved submodel references: %v", [unresolved])) if count(unresolved) > 0

results contains result(0, "AAS-dpp-referenced", bool_status(submodel.id in refs), "DppMetadata is referenced by an AAS") if submodel

# ---- 1. DppMetadata submodel ----
results contains result(1, "STRUCT-submodel", "FAIL", "No submodel with idShort 'DppMetadata'") if count(candidates) == 0

results contains result(1, "STRUCT-submodel", "PASS", "DppMetadata submodel found") if count(candidates) == 1

results contains result(1, "STRUCT-submodel", "FAIL", "More than one submodel with idShort 'DppMetadata'") if count(candidates) > 1

results contains result(1, "SEM-submodel", bool_status(sem_ok(submodel, concat("", [cds, "dppMetadata/1"]))), "submodel semanticId is .../cds/dppMetadata/1") if submodel

# one DUP-<idShort> failure per element name that appears more than once; nothing when all are unique
results contains result(1, sprintf("DUP-%s", [id]), "FAIL", sprintf("idShort '%s' occurs more than once", [id])) if {
	some id in ids
	count([x | some x in els; x.idShort == id]) > 1
}

# ---- 2. mandatory / optional presence ----
# presence_status(mandatory, present): present -> PASS, missing mandatory -> FAIL, missing optional -> SKIP
presence_status(_, true) := "PASS"

presence_status(true, false) := "FAIL"

presence_status(false, false) := "SKIP"

cond_name(true) := "Mandatory"

cond_name(false) := "Optional"

presence_text(true) := "present"

presence_text(false) := "missing"

# one MAND-<attribute> result for every attribute in `attributes`, present or not
results contains result(2, sprintf("MAND-%s", [id]), status, msg) if {
	submodel
	some id, a in attributes
	present := object.get(elements, id, null) != null
	status := presence_status(a.mandatory, present)
	msg := sprintf("%s attribute '%s' %s", [cond_name(a.mandatory), id, presence_text(present)])
}

has_value(e) if {
	is_string(e.value)
	trim_space(e.value) != ""
} else := false

# only for mandatory attributes that exist (elements[id] is undefined for missing ones, which skips the rule)
results contains result(2, sprintf("VAL-%s-nonempty", [id]), bool_status(has_value(e)), sprintf("mandatory attribute '%s' has a non-blank value", [id])) if {
	some id, a in attributes
	a.mandatory
	e := elements[id]
}

# ---- 3. type checks and semanticIds (only for present elements) ----
# every attribute must be a Property with the valueType listed in `attributes`
type_ok(e, a) if {
	e.modelType == "Property"
	e.valueType == a.valueType
} else := false

results contains result(3, sprintf("TYPE-%s", [id]), bool_status(type_ok(e, a)), msg) if {
	some id, a in attributes
	e := elements[id]
	msg := sprintf("modelType=%v valueType=%v, expected Property/%s", [object.get(e, "modelType", "none"), object.get(e, "valueType", "none"), a.valueType])
}

# each attribute's semanticId must be <cds><attribute name>/1
results contains result(3, sprintf("SEM-%s", [id]), bool_status(sem_ok(e, concat("", [cds, id, "/1"]))), sprintf("semanticId is .../cds/%s/1", [id])) if {
	some id, _ in attributes
	e := elements[id]
}

# supplementalSemanticIds required by the spec on granularity (AssetKind) and lastUpdate (updatedAt)
supplemental := {
	"granularity": "https://admin-shell.io/aas/3/2/AssetKind",
	"lastUpdate": "https://admin-shell.io/aas/3/2/AdministrativeInformation/updatedAt",
}

supp_ok(e, expected) if {
	some r in e.supplementalSemanticIds
	some k in r.keys
	k.value == expected
} else := false

results contains result(3, sprintf("SEM-%s-supplemental", [id]), bool_status(supp_ok(elements[id], expected)), sprintf("supplementalSemanticId is %s", [expected])) if {
	some id, expected in supplemental
	elements[id]
}

# ---- 4. granularity enumeration ----
# value must be one of granularity_values (only checked when the granularity element exists)
results contains result(4, "VAL-granularity", bool_status(object.get(elements.granularity, "value", null) in granularity_values), sprintf("granularity = %v, expected Model, Item or Batch", [object.get(elements.granularity, "value", null)])) if elements.granularity

# Table 2: granularity must map to the AAS assetKind of the shell that references the DPP submodel
asset_kind := {"Model": "Type", "Item": "Instance", "Batch": "Batch"}

# shells that reference the DppMetadata submodel
dpp_shells contains sh if {
	some sh in shells
	some r in object.get(sh, "submodels", [])
	some k in r.keys
	k.value == submodel.id
}

kind_ok(sh, g) if {
	sh.assetInformation.assetKind == asset_kind[g]
} else := false

# one result per referencing shell
results contains result(4, "VAL-granularity-assetKind", bool_status(kind_ok(sh, g)), sprintf("granularity %v maps to assetKind %v; shell '%v' has %v", [g, object.get(asset_kind, g, "none"), object.get(sh, "idShort", "?"), object.get(object.get(sh, "assetInformation", {}), "assetKind", "none")])) if {
	g := elements.granularity.value
	some sh in dpp_shells
}

# ---- 5. contentSpecificationIds (SML, optional, section 3.2) ----
# the optional list element; undefined when absent, so the `if csi` checks below only run when it exists
csi := elements.contentSpecificationIds

# its items, or [] when value is missing or not a list
csi_items := v if {
	v := csi.value
	is_array(v)
} else := []

# list shape required by the spec; orderRelevant may be omitted, but if set it must be false
csi_ok(e) if {
	e.modelType == "SubmodelElementList"
	e.typeValueListElement == "Property"
	e.valueTypeListElement == "xs:string"
	object.get(e, "orderRelevant", true) == false
} else := false

results contains result(5, "OPT-contentSpecificationIds", "SKIP", "Optional attribute 'contentSpecificationIds' missing") if {
	submodel
	not csi
}

results contains result(5, "OPT-contentSpecificationIds", "PASS", "Optional attribute 'contentSpecificationIds' present") if csi

results contains result(5, "SML-contentSpecificationIds", bool_status(csi_ok(csi)), "SubmodelElementList of Property/xs:string, orderRelevant=false") if csi

results contains result(5, "SEM-contentSpecificationIds", bool_status(sem_ok(csi, concat("", [cds, "contentSpecificationIds/1"]))), "semanticId is .../cds/contentSpecificationIds/1") if csi

results contains result(5, "SML-contentSpecificationIds-count", bool_status(count(csi_items) >= 1), sprintf("%d item(s), at least 1 required", [count(csi_items)])) if csi

# per-item checks, reported as SML-item-<index> and SEM-item-<index>
item_ok(it) if {
	it.modelType == "Property"
	it.valueType == "xs:string"
	is_string(it.value)
	trim_space(it.value) != ""
} else := false

results contains result(5, sprintf("SML-item-%d", [i]), bool_status(item_ok(it)), "item is Property/xs:string with a non-empty value") if some i, it in csi_items

results contains result(5, sprintf("SEM-item-%d", [i]), bool_status(sem_ok(it, concat("", [cds, "contentSpecificationId/1"]))), "item semanticId is .../cds/contentSpecificationId/1") if some i, it in csi_items

# ---- 6. date-time: well-formed, and timezone required (spec: lastUpdate is UTC-based) ----
# xs:dateTime shape: YYYY-MM-DDThh:mm:ss, optional fraction, optional timezone
dt_re := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?$`

has_tz(v) := regex.match(`(Z|[+-]\d{2}:\d{2})$`, v)

# the RFC 3339 parser needs a timezone, so add Z only for parsing; the missing tz is still reported below
with_tz(v) := v if has_tz(v)

with_tz(v) := concat("", [v, "Z"]) if not has_tz(v)

# calendar/range validation is delegated to the RFC 3339 parser (tz appended when absent)
well_formed(v) if {
	regex.match(dt_re, v)
	time.parse_rfc3339_ns(with_tz(v))
} else := false

# the three lastUpdate checks build on each other: well-formed -> has a timezone -> timezone is UTC
results contains result(6, "DT-lastUpdate-wellformed", bool_status(well_formed(v)), sprintf("lastUpdate = %v", [v])) if {
	v := elements.lastUpdate.value
	is_string(v)
}

results contains result(6, "DT-lastUpdate-timezone", bool_status(has_tz(v)), "timezone designator (Z or +hh:mm) is required") if {
	v := elements.lastUpdate.value
	is_string(v)
	well_formed(v)
}

# spec: "Timestamp UTC-based", so only Z or an offset of zero is accepted
results contains result(6, "DT-lastUpdate-utc", bool_status(regex.match(`(Z|[+-]00:00)$`, v)), "UTC required (Z or +00:00)") if {
	v := elements.lastUpdate.value
	is_string(v)
	well_formed(v)
	has_tz(v)
}

# ---- report ----
# allow is true when nothing FAILed; SKIP (missing optional attribute) does not block it
make_report(rs) := {"allow": count(f) == 0, "failed": count(f), "groups": groups, "results": sort(rs)} if {
	f := [r | some r in rs; r.status == "FAIL"]
}

# METADATA
# title: DPP compliance report
# description: >-
#   The IDTA 02099-1 checks (groups 0-6). Returns allow, failed, groups and results;
#   when allow is true, also the compliance credential (vc, plus vc_jwt or vc_error).
#   Queried as POST /v1/data/dpp/report.
# entrypoint: true
report := with_vc(make_report(results))

# METADATA
# title: DPP compliance report with technical data
# description: >-
#   Same as report, plus the demo-only technical data checks (group 7).
#   Queried as POST /v1/data/dpp/report_tech.
# entrypoint: true
report_tech := with_vc(make_report(results | tech_results))

# ---- 7. technical data (demo only, not part of IDTA 02099-1) ----
# Port of check_technical_data in gaiax_demonstrator: a submodel whose idShort contains "technical"
# (case, spaces, _ and - ignored) with >= 3 elements that carry a value, referenced by an AAS.
# Only evaluated in report_tech; a missing technical submodel fails there, as in the Python checker.
tech_min_elements := 3

# "Technical_Data" / "technical-data" / "TechnicalData" all become "technicaldata"
norm_name(s) := regex.replace(lower(s), `[^a-z0-9]`, "")

# every submodel (other than DppMetadata) whose normalized idShort contains "technical"
tech_candidates := [s |
	some s in all_submodels
	s.id != object.get(submodel, "id", null)
	is_string(s.idShort)
	contains(norm_name(s.idShort), "technical")
]

# element nodes sit at paths [i] / [i, "value", j] / ...; collections and lists are not counted themselves
# (walk() also visits every inner field like semanticId.keys[0]; this filter keeps only real elements:
# odd path length, even positions are array indexes, odd positions are "value")
tech_element(path) if {
	count(path) % 2 == 1
	every i, p in path {
		path_step_ok(i, p)
	}
}

path_step_ok(i, _) if i % 2 == 0

path_step_ok(i, "value") if i % 2 == 1

# "has a value" depends on the element type:
# MultiLanguageProperty -> at least one non-blank text; Range -> min or max set; anything else -> non-empty value
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

# tech_counts[candidate index] = [elements with a value, elements without]
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

# the chosen technical-data submodel; undefined when there are no candidates
tech := tech_candidates[tech_best]

# `submodel` guard: without a DppMetadata submodel the missing-technical-data failure is not reported
tech_results contains result(7, "TECH-submodel", "FAIL", "no submodel with 'technical' in its idShort") if {
	submodel
	count(tech_candidates) == 0
}

tech_results contains result(7, "TECH-submodel", "PASS", sprintf("technical-data submodel '%s' found (%d candidate(s))", [tech.idShort, count(tech_candidates)])) if tech

# TECH-elements: a FAIL when submodelElements is not a list, otherwise the >= tech_min_elements count check
tech_results contains result(7, "TECH-elements", bool_status(is_array(tech.submodelElements)), "submodelElements is missing or not an array") if {
	tech
	not is_array(tech.submodelElements)
}

tech_results contains result(7, "TECH-elements", bool_status(c[0] >= tech_min_elements), sprintf("%d element(s) with a value, %d without; at least %d required", [c[0], c[1], tech_min_elements])) if {
	is_array(tech.submodelElements)
	c := tech_counts[tech_best]
}

tech_results contains result(7, "TECH-ref", bool_status(tech.id in refs), sprintf("'%s' is referenced by an AAS", [tech.idShort])) if tech
