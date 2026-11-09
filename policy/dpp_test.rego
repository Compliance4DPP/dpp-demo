package dpp_test

import data.dpp
import rego.v1

base := {
	"assetAdministrationShells": [{
		"id": "aas-1",
		"idShort": "Shell",
		"assetInformation": {"assetKind": "Instance"},
		"submodels": [{"type": "ModelReference", "keys": [{"type": "Submodel", "value": "sm-dpp"}]}],
	}],
	"submodels": [{
		"id": "sm-dpp",
		"idShort": "DppMetadata",
		"semanticId": sem("dppMetadata"),
		"submodelElements": [
			prop("digitalProductPassportId", "xs:string", "a"),
			prop("uniqueProductIdentifier", "xs:string", "a"),
			supp(prop("granularity", "xs:string", "Item"), "https://admin-shell.io/aas/3/2/AssetKind"),
			prop("dppSchemaVersion", "xs:string", "a"),
			prop("dppStatus", "xs:string", "a"),
			supp(prop("lastUpdate", "xs:dateTime", "2026-10-21T15:53:00Z"), "https://admin-shell.io/aas/3/2/AdministrativeInformation/updatedAt"),
			prop("economicOperatorId", "xs:string", "a"),
		],
	}],
}

sem(name) := {"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": concat("", ["https://admin-shell.io/idta/cds/", name, "/1"])}]}

prop(id, vt, v) := {"idShort": id, "semanticId": sem(id), "modelType": "Property", "valueType": vt, "value": v}

supp(e, v) := object.union(e, {"supplementalSemanticIds": [{"type": "ExternalReference", "keys": [{"type": "GlobalReference", "value": v}]}]})

csi(items) := {
	"idShort": "contentSpecificationIds",
	"semanticId": sem("contentSpecificationIds"),
	"modelType": "SubmodelElementList",
	"typeValueListElement": "Property",
	"valueTypeListElement": "xs:string",
	"orderRelevant": false,
	"value": items,
}

item(v) := {"semanticId": sem("contentSpecificationId"), "modelType": "Property", "valueType": "xs:string", "value": v}

with_csi(items) := json.patch(base, [{"op": "add", "path": "/submodels/0/submodelElements/-", "value": csi(items)}])

with_update(v) := json.patch(base, [{"op": "replace", "path": "/submodels/0/submodelElements/5/value", "value": v}])

test_valid if dpp.report.allow with input as base

test_missing_mandatory if {
	i := json.remove(base, ["submodels/0/submodelElements/4"])
	not dpp.report.allow with input as i
}

test_no_timezone_fails if {
	not dpp.report.allow with input as with_update("2026-10-21T15:53:00")
}

test_bad_calendar_date if {
	not dpp.report.allow with input as with_update("2026-02-30T10:00:00Z")
}

test_zero_offset_ok if dpp.report.allow with input as with_update("2026-10-21T15:53:00+00:00")

test_non_utc_offset_fails if not dpp.report.allow with input as with_update("2026-10-21T15:53:00+02:00")

test_wrong_type if {
	i := json.patch(base, [{"op": "replace", "path": "/submodels/0/submodelElements/0/valueType", "value": "xs:int"}])
	not dpp.report.allow with input as i
}

test_no_submodel if not dpp.report.allow with input as {"submodels": []}

test_bad_granularity if {
	i := json.patch(base, [{"op": "replace", "path": "/submodels/0/submodelElements/2/value", "value": "Lot"}])
	not dpp.report.allow with input as i
}

test_wrong_semantic_id if {
	i := json.patch(base, [{"op": "replace", "path": "/submodels/0/submodelElements/4/semanticId", "value": sem("other")}])
	not dpp.report.allow with input as i
}

test_wrong_submodel_semantic_id if {
	i := json.patch(base, [{"op": "replace", "path": "/submodels/0/semanticId", "value": sem("other")}])
	not dpp.report.allow with input as i
}

test_content_spec_ok if dpp.report.allow with input as with_csi([item("https://example.org/spec/1")])

test_content_spec_empty_list if not dpp.report.allow with input as with_csi([])

test_content_spec_blank_item if not dpp.report.allow with input as with_csi([item("  ")])

test_content_spec_wrong_list_type if {
	i := json.patch(with_csi([item("x")]), [{"op": "replace", "path": "/submodels/0/submodelElements/7/valueTypeListElement", "value": "xs:int"}])
	not dpp.report.allow with input as i
}

test_content_spec_item_semantic_id if {
	i := json.patch(with_csi([item("x")]), [{"op": "replace", "path": "/submodels/0/submodelElements/7/value/0/semanticId", "value": sem("other")}])
	not dpp.report.allow with input as i
}

test_shell_without_asset_information if {
	i := json.remove(base, ["assetAdministrationShells/0/assetInformation"])
	not dpp.report.allow with input as i
}

test_unresolved_reference if {
	i := json.patch(base, [{"op": "replace", "path": "/assetAdministrationShells/0/submodels/0/keys/0/value", "value": "nowhere"}])
	not dpp.report.allow with input as i
}

test_submodel_without_id if {
	i := json.remove(base, ["submodels/0/id"])
	not dpp.report.allow with input as i
}

test_duplicate_element if {
	i := json.patch(base, [{"op": "add", "path": "/submodels/0/submodelElements/-", "value": prop("dppStatus", "xs:string", "b")}])
	not dpp.report.allow with input as i
}

test_two_dpp_submodels if {
	i := json.patch(base, [{"op": "add", "path": "/submodels/-", "value": {"id": "sm-2", "idShort": "DppMetadata"}}])
	not dpp.report.allow with input as i
}

test_blank_mandatory_value if {
	i := json.patch(base, [{"op": "replace", "path": "/submodels/0/submodelElements/3/value", "value": "  "}])
	not dpp.report.allow with input as i
}

test_missing_value_key if {
	i := json.remove(base, ["submodels/0/submodelElements/4/value"])
	not dpp.report.allow with input as i
}

test_granularity_matches_asset_kind if dpp.report.allow with input as with_granularity("Item", "Instance")

test_model_maps_to_type if dpp.report.allow with input as with_granularity("Model", "Type")

test_batch_maps_to_batch if dpp.report.allow with input as with_granularity("Batch", "Batch")

test_granularity_asset_kind_mismatch if not dpp.report.allow with input as with_granularity("Model", "Instance")

test_batch_with_instance_mismatch if not dpp.report.allow with input as with_granularity("Batch", "Instance")

with_granularity(g, kind) := json.patch(base, [
	{"op": "replace", "path": "/submodels/0/submodelElements/2/value", "value": g},
	{"op": "replace", "path": "/assetAdministrationShells/0/assetInformation/assetKind", "value": kind},
])

test_missing_supplemental_granularity if {
	i := json.remove(base, ["submodels/0/submodelElements/2/supplementalSemanticIds"])
	not dpp.report.allow with input as i
}

test_missing_supplemental_last_update if {
	i := json.remove(base, ["submodels/0/submodelElements/5/supplementalSemanticIds"])
	not dpp.report.allow with input as i
}
