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

test_vc_issued_when_valid if {
	vc := dpp.report.vc with input as base with time.now_ns as 0
	vc.validFrom == "1970-01-01T00:00:00Z"
	vc.validUntil == "1970-04-01T00:00:00Z"
	vc.credentialSubject.id == concat("", [vc.id, "#cs"])
	vc.credentialSubject["gx:compliantCredentials"][0].id == "sm-dpp"
	"STRUCT-submodel" in vc.credentialSubject["gx:validatedCriteria"]
}

# throwaway RSA key, generated for this test only
throwaway_key := "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUV2UUlCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktjd2dnU2pBZ0VBQW9JQkFRREMrRVJZYVpUcStMYmQKdTRqOGl5TUY3RnhOclR4aHdLa1RCVGxNK05hTm1Ed3o4bGFnYXlIM3cwaHlPV2VMcjVlOThhcmMvby9ZMnBGZwpRQUg2MDRyZ3ZDZTlkZ3kyQTRuaWVCYks5cUdOSDk1V2krL1RueUxndFFLaFVqeXA4bTIxeDQ3WjkrRTlycXBCCmtEV3VxSDZwd2tDV3ZNSGN5VFYyaEptSkFReDZNUi9tR1F3eWZJZG5DY2tpUzR4YmEwNWh0WEpZMmR1V3pRZ2oKR0prM1hjbWVvS29ZRnhYUEVNa3ZRbm1vWkpFZTRYeHlBUFZER09vZDJnRFlIY0Q4VG5rVVZLblhtNzkxQTArMgpqTmMrRlN4NXJvVXJSVWMzZkN4UWtkd3UxQW44c3BsWSttV3AwVnlTU0xTOFFTZHp0dlE3eEZBMldHaS92MzZICjFBUlgrdmwvQWdNQkFBRUNnZ0VBS0E4dHI4TEZ5NytkaHhLRXMycjlIN2J3UWkrQmRBNUxUdktvVXRYZjg0amYKZWNibG9LRlNnZDg0Yit1MmlEOWtYSzBlYjdqSUlvVEtFTzRsMmYwRUVHZm9mR2VZZGY5bEljaU4ybGJ4VzVjOApMaWtqZGlaQTJzMm1SRElxNzdvMWEzK2NyTkZmRUxibUdZR0JYaWtWbVZpbHY3TDUzQnpsWkx3YTljam00T1lDCkMrVmJBa0p5aUJqVE9RWGw4a3E4dUhQNUZ3b3pXWW1iOFJZalZaUzc4S1lRUDNxcUZoL1cxZG43czJKdnpFb3gKdTYwei9mM21hUCtrWndaQUl3VDM0cm5VSW5JVUNRY21vU2FFNnFZVDc3TDhRSm1tZENUUXNINEo2ZGo3bkxkeQovSkNKOWNEYXVZbmMxVlIxdktvTUFSWTA4Q29VbnZkREU2bXUwbVdWNVFLQmdRRHQrQXdnM3Zsa081ODFtM1JGClJYRHhQTVRZaEpQNGQyS1VNWnRXZmJSZE51UWs3QUdkMnFjVEdsU2JuOC9hcklVdDN2TW5SOUFLODVvaWFuOFgKbldLc0dqcjlNSTJoeGd5eEpGRHUvTVZBeUdWZ3pZdlZoYVFmd1FEbERrcGUwOXpmeVNnQm5UNFZFRHUxUVJWNQpWckxRQldtckVaRkYyRDRvMUIzOEVHL2xuUUtCZ1FEUnZpY1psckM5bEo2VGg1U0FET01Rb1BMUW5zZEttUm1iClVBRHlGV2FXaEoxbzVyWmZIc00zNkhJU2wrems3MlpRNmxGeHluY05lcVg0NWd1bTJMeFB5RDk0MTBPdWJRU3MKNjFHOW9NZGVYV3J5RE9oOHR6OWp0cUZ6UEpUdkJ1WG95VVkxYldCS2NCVk1pQUhoeU15VHRtY0FpMHdiY1ArNwpkQWJsODkyZXl3S0JnUUNwU1pKbk4vZzJnK3VvMlNRSHUzU3pTTVhJT2RpQ0kzdnZiTExTL3EzMnVmdm5BRFhPCk5DUXpCTUkzeTZLMEw1SzJwU3BaUVU4UTIyZEZJSG0xTlBOaTkyNExmQ1RsZHR6RWo4ODdsSHBHc2NqYzZweVYKazNMRVhDcmt5QnZFQmZmVkEyY0Fidm9oUmVFVCttN3FXUER2NTBuYXRxNmYvMHVKQkEwb3FoRFFTUUtCZ0dMegp5akNaVm1McXBmSEdTam9yV2tqekZmdDBuV05GWGI1WGZVcTQ3QUJoS241ZU1uWW41a1FQSnhvOGtPZmxSaWt3Cmt5SVRPaDJpMm9EVnozTjAxMHJ3bDNsa1hhc0NCZTJxeTVLaU1zMERWN3hBZU1LMkZYTHVxN2JaUXc2Tis1UGEKdVlQbFNVTjFJNmRLbDFDMVVrYnlEQ0JHbDY1dW5rZ0swbUpVcmg4SEFvR0FVbEVSWnIreTJZc3lLbTJpV0NNMwoxbVNuVkFRc01wb0NSaW05M01vaTgyYnpVWU9TcGVhR1I2NFZQUlFjTGdYQXZBMjZlYUZKdWZCU05XQmwvUWNwCktCVWx3a0N1STIxdi9aV2o2K2dFcy9ZS3ZpVnpzbmhRMitvNmlQZkVkQjhZZXRqTmoxRWZpZ2pxZmFheXlEcFAKVmxaNStxNnpuOXhZWENoRjI4bVFITkU9Ci0tLS0tRU5EIFBSSVZBVEUgS0VZLS0tLS0K"

test_vc_signed_when_key_set if {
	r := dpp.report with input as base with dpp.vc_key as throwaway_key
	k := crypto.x509.parse_rsa_private_key(throwaway_key)
	io.jwt.verify_ps256(r.vc_jwt, json.marshal({"keys": [{"kty": "RSA", "n": k.n, "e": k.e}]}))
	[h, p, _] := io.jwt.decode(r.vc_jwt)
	h.kid == "did:web:validate.posedio.com#2026-v3"
	p == r.vc
}

test_bad_key_reported if {
	r := dpp.report with input as base with dpp.vc_key as "not a key"
	r.allow
	r.vc_error
	not r.vc_jwt
}

test_unsigned_without_key if {
	r := dpp.report with input as base
	not r.vc_jwt
}

test_no_vc_when_invalid if {
	r := dpp.report with input as with_update("2026-10-21T15:53:00")
	not r.vc
}

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

# ---- technical data ----
tech_sm(els) := {"id": "sm-tech", "idShort": "Technical Data", "submodelElements": els}

tech_ref := {"type": "ModelReference", "keys": [{"type": "Submodel", "value": "sm-tech"}]}

with_tech(els, referenced) := json.patch(base, array.concat(
	[{"op": "add", "path": "/submodels/-", "value": tech_sm(els)}],
	[{"op": "add", "path": "/assetAdministrationShells/0/submodels/-", "value": tech_ref} | referenced],
))

plain(id, v) := {"idShort": id, "modelType": "Property", "valueType": "xs:string", "value": v}

status(check) := {r.status | some r in dpp.report_tech.results; r.check == check}

# one plain, one nested in a collection, one MultiLanguageProperty, one Range, one blank (not counted)
tech_els := [
	plain("manufacturer", "VOLCO"),
	{"idShort": "Geometry", "modelType": "SubmodelElementCollection", "value": [plain("diameter", "10")]},
	{"idShort": "name", "modelType": "MultiLanguageProperty", "value": [{"language": "en", "text": "End mill"}]},
	{"idShort": "rpm", "modelType": "Range", "valueType": "xs:int", "min": "1000"},
	plain("coating", "  "),
]

test_tech_absent_fails_only_in_tech_profile if {
	status("TECH-submodel") == {"FAIL"} with input as base
	not dpp.report_tech.allow with input as base
	dpp.report.allow with input as base
}

test_default_profile_ignores_tech if {
	i := with_tech([plain("a", "1")], false)
	dpp.report.allow with input as i
	not "TECH-ref" in {r.check | some r in dpp.report.results} with input as i
}

test_tech_valid if {
	i := with_tech(tech_els, true)
	dpp.report_tech.allow with input as i
	status("TECH-elements") == {"PASS"} with input as i
	r := [x | some x in dpp.report_tech.results; x.check == "TECH-elements"] with input as i
	contains(r[0].message, "4 element(s) with a value, 1 without")
}

test_tech_too_few_elements if {
	i := with_tech([plain("a", "1"), plain("b", "")], true)
	status("TECH-elements") == {"FAIL"} with input as i
	not dpp.report_tech.allow with input as i
}

test_tech_not_referenced if {
	i := with_tech(tech_els, false)
	status("TECH-ref") == {"FAIL"} with input as i
	not dpp.report_tech.allow with input as i
}

test_tech_elements_not_array if {
	i := with_tech("oops", true)
	status("TECH-elements") == {"FAIL"} with input as i
}
