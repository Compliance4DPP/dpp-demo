{{- /*
  Asset description (Markdown, rendered by the Pontus-X portals). Images are loaded from
  TEMPLATE_IMAGE_BASE_URL, by default the provider's static files; upload this template's
  images/ folder (not in git) there.
*/ -}}
{{- $img := env "TEMPLATE_IMAGE_BASE_URL" | default "https://credentials.ptw.maschinenbau.tu-darmstadt.de/static/" -}}
{{- $product := prop .DPP "ManufacturerProductDesignation" | default (prop .DPP "productDesignation") | default .ID -}}
{{- $so := index .Credentials "service-offering" -}}
![COMPLIANCE4DPP: Automated Compliance for Digital Product Passports]({{$img}}compliance4dpp-banner.jpg)

# Digital Product Passport: {{md $product}}

The DPP of the cutting tool **{{md $product}}** as Asset Administration Shell (AAS JSON,
IDTA 02099-1), checked by the [COMPLIANCE4DPP web demonstrator](https://github.com/Compliance4DPP/dpp-demo),
which issued the verifiable compliance credential below.

## The tool at a glance

| | |
| --- | --- |
| **Product** | {{md $product}} |
{{- with prop .DPP "manufacturer" | default (prop .DPP "ManufacturerName")}}
| **Manufacturer** | {{md .}} |
{{- end}}
{{- with prop .DPP "manufacturerProductCode" | default (prop .DPP "ProductArticleNumberOfManufacturer")}}
| **Product code** | {{md .}} |
{{- end}}
{{- with prop .DPP "toolMaterial" | default (prop .DPP "substrateMaterial")}}
| **Material** | {{md .}} |
{{- end}}
{{- with prop .DPP "coating" | default (prop .DPP "coatingMaterial")}}
| **Coating** | {{md .}} |
{{- end}}
| **DPP id** | {{md .ID}} |
{{- with prop .DPP "lastUpdate"}}
| **Last update** | {{md .}} |
{{- end}}

**Submodels:** {{range $i, $sm := index .DPP "submodels"}}{{if $i}}, {{end}}{{md (index $sm "idShort" | printf "%v")}}{{end}}

## DPP and Asset Administration Shell

Digital Product Passports become mandatory under the EU's ESPR. The
[Asset Administration Shell](https://industrialdigitaltwin.org/) carries them as standardised
submodels that any AAS-capable system can read.

![A Digital Product Passport built from the submodels of an Asset Administration Shell]({{$img}}dpp-via-aas.png)

*Figure: IDTA, [AAS specification](https://github.com/admin-shell-io/aas-specs-metamodel), CC BY 4.0.*

{{with .Compliance -}}
## Compliance credential

{{with index .Doc "credentialSubject"}}{{md (printf "%v" (index . "gx:rulesVersion"))}}, {{len (index . "gx:validatedCriteria")}} criteria passed.{{end}}
{{- if .JWT}} Signed by `{{index .Doc "issuer"}}`, valid until {{md (printf "%.10s" (index .Doc "validUntil"))}}:

```
{{.JWT}}
```
{{- else}} Issued **unsigned** (the demonstrator ran without a signing key).
{{- end}}

{{end -}}
## Gaia-X service offering

A `gx:DigitalServiceOffering` provided by **{{md (.Provider.Name | default .Provider.ID)}}**,
combined with its [participant credential]({{.Provider.CredentialURL}}) into one
[verifiable presentation]({{$so.ID}}.jwt).

## About COMPLIANCE4DPP

![COMPLIANCE4DPP]({{$img}}compliance4dpp-symbol.png)

[COMPLIANCE4DPP](https://www.compliance4dpp.eu/) (Horizon Europe, GA 101298718) makes compliance
an operational capability of Digital Product Passports and Gaia-X data spaces.

![Funded by the European Union]({{$img}}eu-funded.png)

*Funded by the European Union. Views and opinions expressed are however those of the author(s)
only and do not necessarily reflect those of the European Union or the granting authority. Neither
the European Union nor the granting authority can be held responsible for them.*
