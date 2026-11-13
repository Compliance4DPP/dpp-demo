package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// provider is the provider's Gaia-X participant, from its participant credential: a
// presentation (vp+jwt) of enveloped credentials, one of them a gx:LegalPerson, as the
// Gaia-X Loire compliance service expects it. Its signatures are not checked here; the
// compliance service and the catalogue do that.
type provider struct {
	// CredentialURL is where the participant credential is hosted (PROVIDER_CREDENTIAL_URL)
	CredentialURL string
	// ID is the credential subject id of the gx:LegalPerson, the target of gx:providedBy
	ID string
	// Name is its schema:name (or gx:legalName)
	Name string
	// CountryCode and Locality are from its gx:legalAddress
	CountryCode, Locality string
	// Issuer issued the gx:LegalPerson credential
	Issuer string

	// credentials are the presentation's credentials as given, e.g.
	// {"type": "EnvelopedVerifiableCredential", "id": "data:application/vc+jwt,..."}
	credentials []json.RawMessage
}

// fetchProvider downloads and reads the participant credential at url.
func fetchProvider(ctx context.Context, url string) (*provider, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &upstreamError{Status: resp.StatusCode}
	}
	p, err := parseProvider(b)
	if err != nil {
		return nil, err
	}
	p.CredentialURL = url
	return p, nil
}

// parseProvider reads a participant credential: a presentation or a single credential, as
// compact JWS or as JSON-LD.
func parseProvider(b []byte) (*provider, error) {
	doc, err := decodeCredential(strings.TrimSpace(string(b)))
	if err != nil {
		return nil, err
	}
	p := &provider{}
	if vcs, ok := doc["verifiableCredential"]; ok {
		list, _ := vcs.([]any)
		if m, ok := vcs.(map[string]any); ok {
			list = []any{m}
		}
		for _, vc := range list {
			raw, _ := json.Marshal(vc)
			p.credentials = append(p.credentials, raw)
		}
	} else {
		// a single credential: present it as it came
		if s := strings.TrimSpace(string(b)); !strings.HasPrefix(s, "{") {
			raw, _ := json.Marshal(enveloped("vc+jwt", s))
			p.credentials = append(p.credentials, raw)
		} else {
			p.credentials = append(p.credentials, json.RawMessage(s))
		}
	}

	for _, raw := range p.credentials {
		var vc map[string]any
		if err := json.Unmarshal(raw, &vc); err != nil {
			continue
		}
		vc, err := resolveEnveloped(vc)
		if err != nil || !hasType(vc, "gx:LegalPerson") {
			continue
		}
		cs, _ := vc["credentialSubject"].(map[string]any)
		if l, ok := vc["credentialSubject"].([]any); ok && len(l) > 0 {
			cs, _ = l[0].(map[string]any)
		}
		if cs == nil {
			continue
		}
		p.ID, _ = cs["id"].(string)
		p.Name = firstString(cs, "schema:name", "gx:legalName", "name")
		if a, ok := cs["gx:legalAddress"].(map[string]any); ok {
			p.CountryCode = firstString(a, "gx:countryCode", "gx:countrySubdivisionCode")
			p.Locality = firstString(a, "vcard:locality", "gx:locality")
		}
		p.Issuer = issuerID(vc["issuer"])
		break
	}
	if p.ID == "" {
		return nil, errors.New("no gx:LegalPerson credential with a credentialSubject id")
	}
	return p, nil
}

// decodeCredential parses a credential or presentation given as JSON-LD or as compact JWS
// (of which the payload is read, without checking the signature).
func decodeCredential(s string) (map[string]any, error) {
	b, err := credentialJSON(s)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil || doc == nil {
		return nil, errors.New("not a JSON object")
	}
	return doc, nil
}

// credentialJSON returns the JSON of a credential given as JSON-LD or as compact JWS.
func credentialJSON(s string) ([]byte, error) {
	if strings.HasPrefix(s, "{") {
		return []byte(s), nil
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil, errors.New("neither JSON nor a compact JWS")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, fmt.Errorf("JWS payload: %w", err)
	}
	return payload, nil
}

// resolveEnveloped returns the credential inside an EnvelopedVerifiableCredential
// ("data:application/vc+jwt,<jws>"), or vc itself.
func resolveEnveloped(vc map[string]any) (map[string]any, error) {
	if !hasType(vc, "EnvelopedVerifiableCredential") {
		return vc, nil
	}
	id, _ := vc["id"].(string)
	_, jws, ok := strings.Cut(id, ",")
	if !ok || !strings.HasPrefix(id, "data:") {
		return nil, errors.New("enveloped credential without a data: URL")
	}
	return decodeCredential(jws)
}

// enveloped wraps a compact JWS credential for a presentation.
func enveloped(mediaType, jws string) map[string]any {
	return map[string]any{
		"@context": "https://www.w3.org/ns/credentials/v2",
		"type":     "EnvelopedVerifiableCredential",
		"id":       "data:application/" + mediaType + "," + jws,
	}
}

func hasType(doc map[string]any, t string) bool {
	switch v := doc["type"].(type) {
	case string:
		return v == t
	case []any:
		for _, x := range v {
			if x == t {
				return true
			}
		}
	}
	return false
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case map[string]any: // {"@value": "..."}
			if s, ok := v["@value"].(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func issuerID(v any) string {
	if m, ok := v.(map[string]any); ok {
		v = m["id"]
	}
	s, _ := v.(string)
	return s
}
