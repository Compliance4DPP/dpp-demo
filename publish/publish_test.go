package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

// jws returns a compact JWS of payload with a dummy signature (nothing here checks it).
func jws(t *testing.T, payload any) string {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"RS256"}`)) + "." + enc(b) + ".c2ln"
}

// participantJWT is a participant credential shaped like the provider's: a presentation of
// enveloped credentials, one of them the gx:LegalPerson.
func participantJWT(t *testing.T) string {
	legalPerson := map[string]any{
		"@context": []string{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/gaia-x/development#"},
		"id":       "https://provider.example/participant",
		"type":     []string{"VerifiableCredential", "gx:LegalPerson"},
		"issuer":   "did:web:provider.example",
		"credentialSubject": map[string]any{
			"id":              "https://provider.example/participant#cs",
			"schema:name":     "Example Provider",
			"gx:legalAddress": map[string]any{"type": "gx:Address", "gx:countryCode": "DE", "vcard:locality": "Darmstadt"},
		},
	}
	vatID := map[string]any{"type": []string{"VerifiableCredential", "gx:VatID"}, "credentialSubject": map[string]any{"id": "https://provider.example/rn#cs"}}
	return jws(t, map[string]any{
		"type": "VerifiablePresentation",
		"verifiableCredential": []any{
			enveloped("vc+jwt", jws(t, vatID)),
			enveloped("vc+jwt", jws(t, legalPerson)),
		},
	})
}

func TestParseProvider(t *testing.T) {
	p, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "https://provider.example/participant#cs" || p.Name != "Example Provider" || p.Locality != "Darmstadt" || p.CountryCode != "DE" || len(p.credentials) != 2 {
		t.Errorf("provider = %+v", *p)
	}
	if _, err := parseProvider([]byte(jws(t, map[string]any{"type": "VerifiablePresentation", "verifiableCredential": []any{}}))); err == nil {
		t.Error("presentation without gx:LegalPerson: want error")
	}
}

// templateEnv clears the template variables, so the tests see the template's defaults.
func templateEnv(t *testing.T) {
	for _, k := range []string{"TEMPLATE_TERMS_URL", "TEMPLATE_TERMS_SHA256", "TEMPLATE_IMAGE_BASE_URL", "TEMPLATE_SERVICE_OFFERING_URL", "TEMPLATE_DPP_FILE_URL"} {
		t.Setenv(k, "")
	}
}

func TestDefaultTemplate(t *testing.T) {
	templateEnv(t)
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.description == nil {
		t.Fatal("no description.md")
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	prov.CredentialURL = "https://provider.example/participant.jwt"
	comp, err := complianceCredential(json.RawMessage(`{"id":"urn:uuid:1","issuer":"did:web:check.example","type":["VerifiableCredential","gx:ComplianceCredential"],"validFrom":"2026-11-11T00:00:00Z","validUntil":"2027-02-09T00:00:00Z","credentialSubject":{"gx:rulesVersion":"IDTA-02099-1 v1.0","gx:validatedCriteria":["A","B"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	dpp, raw := loadSample(t)
	data := templateData{DPP: dpp, ID: dppID(dpp, raw), Issuer: "did:web:provider.example", Now: "2026-11-11T00:00:00Z", Provider: prov, Compliance: comp, Credentials: map[string]*credential{}}

	if got := tmpl.credentialNames(); len(got) != 1 || got[0] != "service-offering" {
		t.Fatalf("credentials = %v", got)
	}
	b, err := render(tmpl.credentials["service-offering"], data)
	if err != nil {
		t.Fatal(err)
	}
	so, err := newCredential(b, "eyJ.signed.so")
	if err != nil {
		t.Fatal(err)
	}
	data.Credentials["service-offering"] = so
	cs, _ := so.Doc["credentialSubject"].(map[string]any)
	providedBy, _ := cs["gx:providedBy"].(map[string]any)
	if !hasType(so.Doc, "gx:DigitalServiceOffering") || so.ID != "https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/dpp-service-offering" ||
		so.Doc["issuer"] != data.Issuer || providedBy["id"] != prov.ID || cs["name"] != "COMPLIANCE4DPP Digital Product Passport download" {
		t.Errorf("service offering = %s", so.JSON)
	}

	b, err = renderOffering(tmpl, data)
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Metadata struct {
			Name, Description, ProvidedBy string
		} `json:"metadata"`
		Services []any `json:"services"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatalf("offering: %v\n%s", err, b)
	}
	if o.Metadata.Name != "DPP Fictional end mill 6 mm" || o.Metadata.ProvidedBy != prov.CredentialURL || len(o.Services) == 0 {
		t.Errorf("offering = %s", b)
	}
	if got := serviceOfferingURL(b); got != so.ID+".jwt" {
		t.Errorf("serviceOfferingCredential = %q", got)
	}
	for _, want := range []string{
		"# Digital Product Passport: Fictional end mill 6 mm",
		"(https://credentials.ptw.maschinenbau.tu-darmstadt.de/static/compliance4dpp-banner.jpg)",
		"IDTA-02099-1 v1.0, 2 criteria",
		"Issued **unsigned**", // compliance credential given as JSON
		"[verifiable presentation](https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/dpp-service-offering.jwt)", // service offering: a link
		"provided by **Example Provider**",
		"**Submodels:** Nameplate, DppMetadata",
	} {
		if !strings.Contains(o.Metadata.Description, want) {
			t.Errorf("description lacks %q:\n%s", want, o.Metadata.Description)
		}
	}
	// no credential as JSON, and the service offering not even as JWT (it is linked)
	for _, unwanted := range []string{"```json", "eyJ.signed.so"} {
		if strings.Contains(o.Metadata.Description, unwanted) {
			t.Errorf("description contains %q:\n%s", unwanted, o.Metadata.Description)
		}
	}
}

func TestTemplateFuncs(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/dpp/endmill-0001": "example-com-dpp-endmill-0001",
		"urn:example:aas:MT-EM10-001":          "urn-example-aas-mt-em10-001",
		"--A__b--":                             "a-b",
	} {
		if got := funcs["slug"].(func(string) string)(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := funcs["md"].(func(string) string)("a|b *c*\n<x>"); got != `a\|b \*c\* &lt;x&gt;` {
		t.Errorf("md = %q", got)
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

	for _, tc := range []struct{ name, src string }{
		{"non-TEMPLATE_ env", `{"x": {{env "SIGNER_AUTHORIZATION"}}}`},
		{"invalid JSON", `{"x": {{.ID}}}`},
		{"not an object", `["x"]`},
		{"required", `{"x": {{required "TEMPLATE_X" (env "TEMPLATE_X") | json}}}`},
	} {
		write("offering.json", tc.src)
		tmpl, err := loadTemplate(os.DirFS(dir))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := render(tmpl.offering, data); err == nil {
			t.Errorf("%s: want error", tc.name)
		}
	}
}

// fakeAPI serves POST paths with fixed responses, checks the Authorization header and records
// the request bodies by path.
func fakeAPI(t *testing.T, auth string, status int, responses map[string]string) (*httptest.Server, map[string][]byte) {
	t.Helper()
	got := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res, ok := responses[r.URL.Path]
		if r.Method != http.MethodPost || !ok {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if h := r.Header.Get("Authorization"); h != auth {
			t.Errorf("%s: Authorization = %q, want %q", r.URL.Path, h, auth)
		}
		got[r.URL.Path], _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, res)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// fakeSigner signs credentials and presentations.
func fakeSigner(t *testing.T) (*httptest.Server, map[string][]byte) {
	return fakeAPI(t, "Basic ZGVtbzp0ZXN0", 200, map[string]string{
		"/sdbs/v1/signVC": `{"jwtld":"eyJ.signed.vc"}`,
		"/sdbs/v1/signVP": `{"jwtld":"eyJ.signed.vp"}`,
	})
}

// fakeParticipant serves the participant credential.
func fakeParticipant(t *testing.T) *httptest.Server {
	body := participantJWT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func testPublisher(t *testing.T, signerURL, gatewayURL string) *publisher {
	templateEnv(t)
	return &publisher{
		dppDir:      "../samples",
		templateDir: "templates",
		issuer:      "did:web:provider.example",
		providerURL: fakeParticipant(t).URL + "/participant.jwt",
		timeout:     5 * time.Second,
		maxBody:     1 << 20,
		signer:      &apiClient{name: "signer", base: signerURL, auth: "Basic ZGVtbzp0ZXN0"},
		gateway:     &apiClient{name: "gateway", base: gatewayURL, auth: bearer("tok")},
	}
}

func post(t *testing.T, h http.Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/publish", strings.NewReader(body)))
	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("response %d is not JSON: %s", rec.Code, rec.Body)
	}
	return rec.Code, res
}

func TestPublish(t *testing.T) {
	signer, signerReq := fakeSigner(t)
	// GATEWAY_URL with a path prefix and trailing slash, as in the hosted gateway
	gateway, gatewayReq := fakeAPI(t, "Bearer tok", 200, map[string]string{"/grpc/CreateOffering": `{"id":["did:ope:123"]}`})
	h := testPublisher(t, signer.URL, gateway.URL+"/grpc/").routes()

	compliance := jws(t, map[string]any{"id": "urn:uuid:c1", "issuer": "did:web:check.example", "type": []string{"VerifiableCredential", "gx:ComplianceCredential"}})
	body, _ := json.Marshal(map[string]any{"template": "default", "dpp_name": "valid_dpp", "compliance_credential": compliance})
	code, res := post(t, h, string(body))
	if code != 200 {
		t.Fatalf("status %d: %v", code, res)
	}
	if ids, _ := res["offering_ids"].([]any); len(ids) != 1 || ids[0] != "did:ope:123" {
		t.Errorf("offering_ids = %v", res["offering_ids"])
	}
	if c, _ := res["credentials"].(map[string]any); c["service-offering"] != "eyJ.signed.vc" {
		t.Errorf("credentials = %v", res["credentials"])
	}
	if res["presentation"] != "eyJ.signed.vp" || res["presentation_url"] != "https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/dpp-service-offering.jwt" {
		t.Errorf("presentation = %v at %v", res["presentation"], res["presentation_url"])
	}

	var vcReq struct {
		JSONLD map[string]any `json:"jsonld"`
	}
	if err := json.Unmarshal(signerReq["/sdbs/v1/signVC"], &vcReq); err != nil || vcReq.JSONLD["issuer"] != "did:web:provider.example" {
		t.Errorf("signVC request = %s", signerReq["/sdbs/v1/signVC"])
	}
	// the presentation: the participant's two credentials, then the signed service offering
	var vpReq struct {
		JSONLD struct {
			VCs []struct{ ID string } `json:"verifiableCredential"`
		} `json:"jsonld"`
	}
	if err := json.Unmarshal(signerReq["/sdbs/v1/signVP"], &vpReq); err != nil || len(vpReq.JSONLD.VCs) != 3 ||
		!strings.HasPrefix(vpReq.JSONLD.VCs[1].ID, "data:application/vc+jwt,") || vpReq.JSONLD.VCs[2].ID != "data:application/vc+jwt,eyJ.signed.vc" {
		t.Errorf("signVP request = %s", signerReq["/sdbs/v1/signVP"])
	}
	var greq struct {
		Offerings []struct {
			PontusxOffering struct {
				Metadata struct{ Description string } `json:"metadata"`
			} `json:"pontusxOffering"`
		} `json:"offerings"`
	}
	if err := json.Unmarshal(gatewayReq["/grpc/CreateOffering"], &greq); err != nil || len(greq.Offerings) != 1 ||
		!strings.Contains(greq.Offerings[0].PontusxOffering.Metadata.Description, compliance) {
		t.Errorf("gateway request lacks the compliance credential: %s", gatewayReq["/grpc/CreateOffering"])
	}
}

func TestPublishErrors(t *testing.T) {
	signer, _ := fakeSigner(t)
	// what nginx in front of the hosted gateway answers without a valid token
	denied, _ := fakeAPI(t, "Bearer tok", 401, map[string]string{"/CreateOffering": "<html><title>401 Authorization Required</title></html>"})
	rejected, _ := fakeAPI(t, "Bearer tok", 400, map[string]string{"/CreateOffering": `{"code":3,"message":"metadata.name is required"}`})

	for _, tc := range []struct {
		name, gateway, body string
		status              int
		msg                 string
	}{
		{"unknown template", denied.URL, `{"template":"nope","dpp_name":"valid_dpp"}`, 404, "template nope"},
		{"path in name", denied.URL, `{"template":"../x","dpp_name":"valid_dpp"}`, 400, "template"},
		{"no dpp", denied.URL, `{"template":"default"}`, 400, "dpp"},
		{"both dpps", denied.URL, `{"template":"default","dpp_name":"valid_dpp","dpp":{}}`, 400, "not both"},
		{"unknown field", denied.URL, `{"template":"default","dpp_json":"e30="}`, 400, "unknown field"},
		{"bad compliance credential", denied.URL, `{"template":"default","dpp_name":"valid_dpp","compliance_credential":"x.y"}`, 400, "compliance_credential"},
		{"gateway auth", denied.URL, `{"template":"default","dpp_name":"valid_dpp"}`, 502, "gateway: HTTP 401 Unauthorized"},
		{"gateway rejects", rejected.URL, `{"template":"default","dpp":{}}`, 502, "metadata.name is required"},
	} {
		code, res := post(t, testPublisher(t, signer.URL, tc.gateway).routes(), tc.body)
		msg, _ := res["error"].(string)
		if code != tc.status || !strings.Contains(msg, tc.msg) {
			t.Errorf("%s: %d %q, want %d containing %q", tc.name, code, msg, tc.status, tc.msg)
		}
	}

	// own terms without their hash fail before anything is signed
	t.Run("terms without hash", func(t *testing.T) {
		signer, signed := fakeSigner(t)
		p := testPublisher(t, signer.URL, denied.URL)
		t.Setenv("TEMPLATE_TERMS_URL", "https://provider.example/terms.pdf")
		code, res := post(t, p.routes(), `{"template":"default","dpp_name":"valid_dpp"}`)
		if msg, _ := res["error"].(string); code != 422 || !strings.Contains(msg, "TEMPLATE_TERMS_SHA256 (with TEMPLATE_TERMS_URL) is required") || len(signed) != 0 {
			t.Errorf("%d %q, signer called %d times", code, msg, len(signed))
		}
	})
}

// By default every offering delivers the one shared sample DPP; with TEMPLATE_DPP_FILE_URL the
// offering delivers that file. The description does not mention it either way.
func TestDPPFile(t *testing.T) {
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	dpp, raw := loadSample(t)
	for _, tc := range []struct{ env, url string }{
		{"", "https://credentials.ptw.maschinenbau.tu-darmstadt.de/static/tool_dpp.json"},
		{"https://provider.example/dpp/x.json", "https://provider.example/dpp/x.json"},
	} {
		templateEnv(t)
		t.Setenv("TEMPLATE_DPP_FILE_URL", tc.env)
		data := templateData{DPP: dpp, ID: dppID(dpp, raw), Provider: prov, Credentials: map[string]*credential{}}
		b, err := render(tmpl.credentials["service-offering"], data)
		if err != nil {
			t.Fatal(err)
		}
		if data.Credentials["service-offering"], err = newCredential(b, ""); err != nil {
			t.Fatal(err)
		}
		b, err = renderOffering(tmpl, data)
		if err != nil {
			t.Fatal(err)
		}
		var o struct {
			Metadata struct{ Description string }
			Services []struct {
				Files []struct{ URL string }
			}
		}
		if err := json.Unmarshal(b, &o); err != nil {
			t.Fatal(err)
		}
		if got := o.Services[0].Files[0].URL; got != tc.url {
			t.Errorf("TEMPLATE_DPP_FILE_URL=%q: file %q, want %q", tc.env, got, tc.url)
		}
		if strings.Contains(o.Metadata.Description, "tool_dpp.json") {
			t.Errorf("TEMPLATE_DPP_FILE_URL=%q: description mentions the shared file", tc.env)
		}
	}
}

// The shared demo DPP: its Nameplate uses MultiLanguageProperties and its materials are in
// MaterialComposition; the description's table reads them.
func TestToolDPPDescription(t *testing.T) {
	templateEnv(t)
	raw, err := os.ReadFile("../samples/tool_dpp.json")
	if err != nil {
		t.Skip("samples/tool_dpp.json not present")
	}
	var dpp map[string]any
	if err := json.Unmarshal(raw, &dpp); err != nil {
		t.Fatal(err)
	}
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	data := templateData{DPP: dpp, ID: dppID(dpp, raw), Provider: prov, Credentials: map[string]*credential{}}
	b, err := render(tmpl.credentials["service-offering"], data)
	if err != nil {
		t.Fatal(err)
	}
	if data.Credentials["service-offering"], err = newCredential(b, ""); err != nil {
		t.Fatal(err)
	}
	desc, err := renderText(tmpl.description, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Digital Product Passport: Solid carbide end mill 10 mm, 4 flutes",
		"| **Manufacturer** | PTW TU Darmstadt |",
		"| **Product code** | PTW-EM10-Z4-ALTIN |",
		"| **Material** | Fine-grain cemented carbide (WC with 10 % Co binder) |",
		"| **Coating** | AlTiN (PVD) |",
		"| **DPP id** | https://compliance4dpp.eu/dpp/PTW-EM10-Z4-0001 |",
	} {
		if !strings.Contains(desc, want) {
			t.Errorf("description lacks %q", want)
		}
	}
}

// The default terms are deltaDAO's Pontus-X portal terms, pinned to a commit with their hash.
func TestDefaultTerms(t *testing.T) {
	templateEnv(t)
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	dpp, raw := loadSample(t)
	b, err := render(tmpl.credentials["service-offering"], templateData{DPP: dpp, ID: dppID(dpp, raw), Provider: prov})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"gx:url": {"@id": "https://raw.githubusercontent.com/deltaDAO/mvg-portal/646d9c55ac00e9390add0aaab2516dd5e231797a/content/pages/terms.md"}`,
		`"gx:hash": "eaedd43e132657b195b51c90afc5d47ebe236feedaf641354c9aa6b2d2890550"`,
	} {
		if strings.Count(string(b), want) != 2 { // service and usage terms
			t.Errorf("service offering lacks %s twice:\n%s", want, b)
		}
	}
}

// signedLike returns a compact JWS of payload with the size of a real RSA (2048 bit) signature.
func signedLike(payload any) string {
	b, _ := json.Marshal(payload)
	enc := base64.RawURLEncoding.EncodeToString
	header := `{"alg":"RS256","cty":"vc+ld","iat":1794585650152,"iss":"did:web:credentials.ptw.maschinenbau.tu-darmstadt.de","kid":"did:web:credentials.ptw.maschinenbau.tu-darmstadt.de#X509-JWK2020","typ":"vc+ld+jwt"}`
	return enc([]byte(header)) + "." + enc(b) + "." + strings.Repeat("s", 342)
}

// The ocean node indexes a DDO only up to a size: the gateway refuses a DDO whose signed form
// exceeds 24418 characters. A first publish of tool_dpp.json with an offering of 16922 bytes
// came to 25807, which puts the limit at an offering of about 15880 bytes. With credentials of
// realistic size (the check's 70 criteria, RSA signatures), the rendered offering must stay
// clearly below.
func TestOfferingSize(t *testing.T) {
	const maxOffering = 15000
	templateEnv(t)
	raw, err := os.ReadFile("../samples/tool_dpp.json")
	if err != nil {
		t.Skip("samples/tool_dpp.json not present")
	}
	var dpp map[string]any
	if err := json.Unmarshal(raw, &dpp); err != nil {
		t.Fatal(err)
	}
	criteria := make([]string, 70)
	for i := range criteria {
		criteria[i] = fmt.Sprintf("VAL-dppSchemaVersion-%02d", i) // the length of the real ones (about 25)
	}
	compliance := signedLike(map[string]any{
		"@context":   []string{"https://www.w3.org/ns/credentials/v2", "https://w3id.org/gaia-x/2.5.0#"},
		"type":       []string{"VerifiableCredential", "gx:ComplianceCredential"},
		"id":         "urn:uuid:62191bb7-9416-4562-997b-1a3daeb03cdd",
		"issuer":     "did:web:validate.posedio.com",
		"validFrom":  "2026-11-13T15:26:15.899766274Z",
		"validUntil": "2027-02-11T15:26:15.899766274Z",
		"credentialSubject": map[string]any{
			"id":                      "urn:uuid:62191bb7-9416-4562-997b-1a3daeb03cdd#cs",
			"gx:engineVersion":        "1.22.0",
			"gx:rulesVersion":         "IDTA-02099-1 v1.0",
			"gx:validatedCriteria":    criteria,
			"gx:compliantCredentials": []any{map[string]any{"id": "https://compliance4dpp.eu//ids/sm/5171_0101_1162_5088", "gx:credentialType": "DppMetadata", "digestSRI": "sha256-" + strings.Repeat("a", 64)}},
		},
	})
	jb, _ := json.Marshal(compliance)
	comp, err := complianceCredential(jb)
	if err != nil {
		t.Fatal(err)
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	prov.CredentialURL = "https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/participant.jwt"
	prov.Name = "PTW TU Darmstadt"
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	data := templateData{DPP: dpp, ID: dppID(dpp, raw), Issuer: "did:web:credentials.ptw.maschinenbau.tu-darmstadt.de", Now: "2026-11-13T17:52:21Z", Provider: prov, Compliance: comp, Credentials: map[string]*credential{}}
	so, err := render(tmpl.credentials["service-offering"], data)
	if err != nil {
		t.Fatal(err)
	}
	var soDoc any
	_ = json.Unmarshal(so, &soDoc)
	if data.Credentials["service-offering"], err = newCredential(so, signedLike(soDoc)); err != nil {
		t.Fatal(err)
	}
	b, err := renderOffering(tmpl, data)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("offering %d bytes (limit about 15880, test maximum %d)", len(b), maxOffering)
	if len(b) > maxOffering {
		t.Errorf("offering is %d bytes, more than %d: the ocean node would not index the DDO", len(b), maxOffering)
	}
}

// The service offering is the same for every DPP (one hosted presentation serves all offerings):
// rendered for two different passports, it differs only in validFrom.
func TestServiceOfferingIsShared(t *testing.T) {
	templateEnv(t)
	tmpl, err := loadTemplate(os.DirFS("templates/default"))
	if err != nil {
		t.Fatal(err)
	}
	prov, err := parseProvider([]byte(participantJWT(t)))
	if err != nil {
		t.Fatal(err)
	}
	var docs []map[string]any
	for i, f := range []string{"../samples/valid_dpp.json", "../samples/valid_tech_desc_dpp.json"} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var dpp map[string]any
		_ = json.Unmarshal(raw, &dpp)
		b, err := render(tmpl.credentials["service-offering"], templateData{DPP: dpp, ID: dppID(dpp, raw), Issuer: "did:web:provider.example", Now: fmt.Sprintf("2026-11-1%dT00:00:00Z", i), Provider: prov})
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		_ = json.Unmarshal(b, &doc)
		delete(doc, "validFrom")
		docs = append(docs, doc)
	}
	a, _ := json.Marshal(docs[0])
	b, _ := json.Marshal(docs[1])
	if string(a) != string(b) {
		t.Errorf("service offering differs between DPPs:\n%s\n%s", a, b)
	}
	if docs[0]["id"] != "https://credentials.ptw.maschinenbau.tu-darmstadt.de/sd/dpp-service-offering" {
		t.Errorf("id = %v", docs[0]["id"])
	}
	t.Setenv("TEMPLATE_SERVICE_OFFERING_URL", "https://provider.example/sd/so.jwt")
	raw, _ := os.ReadFile("../samples/valid_dpp.json")
	var dpp map[string]any
	_ = json.Unmarshal(raw, &dpp)
	out, err := render(tmpl.credentials["service-offering"], templateData{DPP: dpp, ID: dppID(dpp, raw), Provider: prov})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"id": "https://provider.example/sd/so"`) {
		t.Errorf("TEMPLATE_SERVICE_OFFERING_URL not used for the id:\n%s", out)
	}
}
