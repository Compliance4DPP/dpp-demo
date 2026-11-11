package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"dpp-demo/publish/gen/gatewayv3"
)

func loadSample(t *testing.T) (map[string]any, []byte) {
	t.Helper()
	raw, err := os.ReadFile("../samples/valid_dpp.json")
	if err != nil {
		t.Fatal(err)
	}
	var dpp map[string]any
	if err := json.Unmarshal(raw, &dpp); err != nil {
		t.Fatal(err)
	}
	return dpp, raw
}

func TestDPPID(t *testing.T) {
	dpp, raw := loadSample(t)
	if got, want := dppID(dpp, raw), "https://example.com/dpp/endmill-0001"; got != want {
		t.Errorf("dppID = %q, want %q", got, want)
	}
	if got := dppID(map[string]any{}, []byte("{}")); got[:7] != "sha256-" {
		t.Errorf("dppID of empty doc = %q, want hash", got)
	}
}

func TestDefaultTemplate(t *testing.T) {
	t.Setenv("TEMPLATE_PROVIDED_BY", "https://example.org/.well-known/gx_legalperson.jwt")
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	dpp, raw := loadSample(t)
	data := templateData{DPP: dpp, ID: dppID(dpp, raw), Issuer: "did:web:example.org", Now: "2026-11-11T00:00:00Z"}

	b, err := render(tmpl.offering, data)
	if err != nil {
		t.Fatal(err)
	}
	o := &gatewayv3.PontusxOffering{}
	if err := protojson.Unmarshal(b, o); err != nil {
		t.Fatalf("offering is no PontusxOffering: %v\n%s", err, b)
	}
	if got := o.GetMetadata().GetName(); got != "DPP Fictional end mill 6 mm" {
		t.Errorf("name = %q", got)
	}
	if got := o.GetMetadata().GetProvidedBy(); got != "https://example.org/.well-known/gx_legalperson.jwt" {
		t.Errorf("providedBy = %q", got)
	}

	if got := tmpl.credentialNames(); len(got) != 1 || got[0] != "service-offering" {
		t.Fatalf("credentials = %v", got)
	}
	b, err = render(tmpl.credentials["service-offering"], data)
	if err != nil {
		t.Fatal(err)
	}
	var vc map[string]any
	if err := json.Unmarshal(b, &vc); err != nil {
		t.Fatal(err)
	}
	if id, _ := vc["id"].(string); !strings.HasPrefix(id, "urn:uuid:") || len(id) != 45 || vc["issuer"] != data.Issuer {
		t.Errorf("vc id/issuer = %v / %v", vc["id"], vc["issuer"])
	}
}

func TestTemplateErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(dir+"/"+name, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data := templateData{DPP: map[string]any{}}

	write("offering.json", `{"x": {{env "SIGNER_AUTHORIZATION"}}}`)
	tmpl, err := loadTemplate(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := render(tmpl.offering, data); err == nil {
		t.Error("non-TEMPLATE_ env: want error")
	}

	write("offering.json", `{"x": {{.ID}}}`)
	tmpl, _ = loadTemplate(os.DirFS(dir))
	if _, err := render(tmpl.offering, data); err == nil {
		t.Error("invalid JSON output: want error")
	}
}
