package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"time"
)

// validName is what DPP, template and credential names may look like (no paths).
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type publisher struct {
	dppDir, templateDir string
	issuer              string
	providerURL         string // PROVIDER_CREDENTIAL_URL; "" for none
	timeout             time.Duration
	maxBody             int64

	signer, gateway *apiClient
}

// publishRequest is the body of POST /publish.
type publishRequest struct {
	// Name of a template directory in TEMPLATE_DIR, e.g. "default"
	Template string `json:"template"`
	// Name of a DPP file in DPP_DIR without .json, e.g. "valid_dpp"; or the DPP itself in dpp
	DPPName string          `json:"dpp_name,omitempty"`
	DPP     json.RawMessage `json:"dpp,omitempty"`
	// The DPP's compliance credential from the check: compact JWS (vc_jwt) or JSON-LD (vc)
	ComplianceCredential json.RawMessage `json:"compliance_credential,omitempty"`
}

// publishResponse is the answer of POST /publish.
type publishResponse struct {
	// Identifier(s) of the published offering, e.g. did:ope:...
	OfferingIDs []string `json:"offering_ids"`
	// The signed credentials as compact JWS, by credential template name
	Credentials map[string]string `json:"credentials"`
	// The signed credentials together with the provider's participant credentials, as one
	// presentation (vp+jwt): the input of the Gaia-X compliance service, to host at
	// presentation_url
	Presentation string `json:"presentation,omitempty"`
	// Where the offering expects the presentation: its
	// additionalInformation.gaiaX.serviceOfferingCredential
	PresentationURL string `json:"presentation_url,omitempty"`
}

func (p *publisher) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /publish", p.handlePublish)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func (p *publisher) handlePublish(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, p.maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, &httpError{http.StatusBadRequest, "request: " + err.Error()})
		return
	}
	res, err := p.publish(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (p *publisher) publish(ctx context.Context, req publishRequest) (*publishResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	if !validName.MatchString(req.Template) {
		return nil, &httpError{http.StatusBadRequest, "template: missing or invalid name"}
	}
	tmpl, err := loadTemplate(os.DirFS(p.templateDir + "/" + req.Template))
	if err != nil {
		return nil, fileError("template "+req.Template, err)
	}

	raw, err := p.loadDPP(req)
	if err != nil {
		return nil, err
	}
	var dpp map[string]any
	if err := json.Unmarshal(raw, &dpp); err != nil {
		return nil, &httpError{http.StatusBadRequest, "dpp: not a JSON object: " + err.Error()}
	}
	compliance, err := complianceCredential(req.ComplianceCredential)
	if err != nil {
		return nil, &httpError{http.StatusBadRequest, "compliance_credential: " + err.Error()}
	}

	id := dppID(dpp, raw)
	log := slog.With("dpp", id, "template", req.Template)
	data := templateData{DPP: dpp, ID: id, Issuer: p.issuer, Now: time.Now().UTC().Format(time.RFC3339), Compliance: compliance}
	if p.providerURL != "" {
		if data.Provider, err = fetchProvider(ctx, p.providerURL); err != nil {
			return nil, upstream("participant credential "+p.providerURL, err)
		}
	}

	// Render everything first, so a broken template fails before anything is signed or
	// published: the description and the offering with the credentials still unsigned.
	creds := map[string][]byte{}
	data.Credentials = map[string]*credential{}
	for _, name := range tmpl.credentialNames() {
		if creds[name], err = render(tmpl.credentials[name], data); err != nil {
			return nil, templateError(err)
		}
		if data.Credentials[name], err = newCredential(creds[name], ""); err != nil {
			return nil, templateError(err)
		}
	}
	if _, err := renderOffering(tmpl, data); err != nil {
		return nil, templateError(err)
	}

	// The participant's credentials and the signed ones make up the presentation.
	var presented []any
	if data.Provider != nil {
		for _, c := range data.Provider.credentials {
			presented = append(presented, c)
		}
	}
	signed := map[string]string{}
	for _, name := range tmpl.credentialNames() {
		jwt, err := p.sign(ctx, "sdbs/v1/signVC", map[string]any{"jsonld": json.RawMessage(creds[name])})
		if err != nil {
			return nil, upstream("signer: credential "+name, err)
		}
		signed[name] = jwt
		data.Credentials[name].JWT = jwt
		// the signer signs as vc+ld+jwt, but the Gaia-X compliance service only decodes
		// enveloped credentials with the media type vc+jwt
		presented = append(presented, enveloped("vc+jwt", jwt))
		log.Info("credential signed", "credential", name)
	}
	var presentation string
	if len(signed) > 0 {
		vp := map[string]any{
			"@context":             []string{"https://www.w3.org/ns/credentials/v2"},
			"type":                 "VerifiablePresentation",
			"verifiableCredential": presented,
		}
		if presentation, err = p.sign(ctx, "sdbs/v1/signVP", map[string]any{"jsonld": vp}); err != nil {
			return nil, upstream("signer: presentation", err)
		}
		log.Info("presentation signed", "credentials", len(presented))
	}

	offering, err := renderOffering(tmpl, data)
	if err != nil {
		return nil, templateError(err)
	}
	var res struct {
		ID []string `json:"id"`
	}
	body := map[string]any{"offerings": []any{map[string]json.RawMessage{"pontusxOffering": offering}}}
	if err := p.gateway.post(ctx, "CreateOffering", body, &res); err != nil {
		return nil, upstream("gateway", err)
	}
	log.Info("offering published", "ids", res.ID)

	return &publishResponse{
		OfferingIDs:     res.ID,
		Credentials:     signed,
		Presentation:    presentation,
		PresentationURL: serviceOfferingURL(offering),
	}, nil
}

// sign has the signer sign a credential or presentation and returns it as compact JWS.
func (p *publisher) sign(ctx context.Context, path string, body any) (string, error) {
	var res struct {
		JWT string `json:"jwtld"`
	}
	if err := p.signer.post(ctx, path, body, &res); err != nil {
		return "", err
	}
	if res.JWT == "" {
		return "", errors.New("response holds no jwtld")
	}
	return res.JWT, nil
}

// renderOffering renders description.md, if any, and offering.json.
func renderOffering(tmpl *offeringTemplate, data templateData) ([]byte, error) {
	if tmpl.description != nil {
		d, err := renderText(tmpl.description, data)
		if err != nil {
			return nil, err
		}
		data.Description = d
	}
	return render(tmpl.offering, data)
}

// complianceCredential reads the compliance credential of a request: a JSON string with the
// compact JWS, or the credential as JSON object.
func complianceCredential(raw json.RawMessage) (*credential, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var jws string
	if json.Unmarshal(raw, &jws) == nil {
		b, err := credentialJSON(jws)
		if err != nil {
			return nil, err
		}
		return newCredential(b, jws)
	}
	return newCredential(raw, "")
}

// serviceOfferingURL returns additionalInformation.gaiaX.serviceOfferingCredential of an offering.
func serviceOfferingURL(offering []byte) string {
	var o struct {
		AdditionalInformation struct {
			GaiaX struct {
				URL string `json:"serviceOfferingCredential"`
			} `json:"gaiaX"`
		} `json:"additionalInformation"`
	}
	_ = json.Unmarshal(offering, &o)
	return o.AdditionalInformation.GaiaX.URL
}

func templateError(err error) error {
	return &httpError{http.StatusUnprocessableEntity, "template: " + err.Error()}
}

func (p *publisher) loadDPP(req publishRequest) ([]byte, error) {
	switch {
	case len(req.DPP) > 0 && req.DPPName != "":
		return nil, &httpError{http.StatusBadRequest, "dpp: set dpp_name or dpp, not both"}
	case len(req.DPP) > 0:
		return req.DPP, nil
	case req.DPPName != "":
		if !validName.MatchString(req.DPPName) {
			return nil, &httpError{http.StatusBadRequest, "dpp_name: invalid name"}
		}
		b, err := os.ReadFile(p.dppDir + "/" + req.DPPName + ".json")
		if err != nil {
			return nil, fileError("dpp "+req.DPPName, err)
		}
		return b, nil
	default:
		return nil, &httpError{http.StatusBadRequest, "dpp: set dpp_name or dpp"}
	}
}

// dppID returns the passport's digitalProductPassportId, else the first shell's id, else a hash
// of the document.
func dppID(dpp map[string]any, raw []byte) string {
	if v, ok := findProperty(dpp["submodels"], "digitalProductPassportId"); ok && v != "" {
		return v
	}
	if shells, ok := dpp["assetAdministrationShells"].([]any); ok && len(shells) > 0 {
		if s, ok := shells[0].(map[string]any); ok {
			if id, ok := s["id"].(string); ok && id != "" {
				return id
			}
		}
	}
	sum := sha256.Sum256(raw)
	return "sha256-" + hex.EncodeToString(sum[:16])
}

// httpError is an error with the HTTP status to answer it with.
type httpError struct {
	Status  int
	Message string
}

func (e *httpError) Error() string { return e.Message }

// upstream wraps an error of a dependency: 504 when the publish timeout ran out, else 502.
func upstream(step string, err error) error {
	status := http.StatusBadGateway
	if isTimeout(err) {
		status = http.StatusGatewayTimeout
	}
	return &httpError{status, step + ": " + err.Error()}
}

func fileError(what string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return &httpError{http.StatusNotFound, what + ": not found"}
	}
	return &httpError{http.StatusUnprocessableEntity, what + ": " + err.Error()}
}

func writeError(w http.ResponseWriter, err error) {
	var he *httpError
	if !errors.As(err, &he) {
		he = &httpError{http.StatusInternalServerError, err.Error()}
	}
	if he.Status >= 500 {
		slog.Warn("publish failed", "status", he.Status, "err", he.Message)
	}
	writeJSON(w, he.Status, map[string]string{"error": he.Message})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
