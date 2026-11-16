package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"text/template"
)

// A template directory holds
//
//	offering.json              PontusxOffering of the Ecosystems Gateway, as in its HTTP/JSON API
//	description.md             optional: Markdown, available to offering.json as .Description
//	credentials/<name>.json    Verifiable Credentials (JSON-LD) to sign, zero or more
//
// The offering is complete in itself: file and credential URLs are fixed in the template, as
// the service hosts nothing.
//
// All files are Go text/templates rendered with templateData and the functions in funcs. The
// credentials are rendered first; description.md and offering.json then also see them signed,
// as .Credentials.
type offeringTemplate struct {
	offering    *template.Template
	description *template.Template            // nil without description.md
	credentials map[string]*template.Template // by name
}

// credentialNames returns the credential names in a stable order.
func (t *offeringTemplate) credentialNames() []string {
	names := make([]string, 0, len(t.credentials))
	for n := range t.credentials {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// templateData is what templates see as ".".
type templateData struct {
	// DPP is the parsed AAS JSON of the passport
	DPP map[string]any
	// ID identifies the passport: its digitalProductPassportId, else the first shell's id
	ID string
	// Issuer is ISSUER_DID, the DID the signer signs with
	Issuer string
	// Now is the time of the request, RFC 3339 in UTC
	Now string
	// Provider is the provider's Gaia-X participant (PROVIDER_CREDENTIAL_URL); nil without it
	Provider *provider
	// Compliance is the DPP's compliance credential of the request; nil without it
	Compliance *credential
	// Credentials are the template's credentials by name, signed (in description.md and
	// offering.json only)
	Credentials map[string]*credential
	// Description is the rendered description.md (in offering.json only)
	Description string
}

// credential is a verifiable credential as templates see it.
type credential struct {
	// ID is the credential's id
	ID string
	// JSON is the credential, indented, e.g. for a code block
	JSON string
	// JWT is the signed credential as compact JWS; "" if unsigned
	JWT string
	// Doc is the parsed credential
	Doc map[string]any
}

// newCredential parses a JSON-LD credential.
func newCredential(b []byte, jwt string) (*credential, error) {
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil || doc == nil {
		return nil, errors.New("not a JSON object")
	}
	// indent the bytes as given, so the key order stays the credential's own
	var buf bytes.Buffer
	if err := json.Indent(&buf, b, "", "  "); err != nil {
		return nil, err
	}
	id, _ := doc["id"].(string)
	return &credential{ID: id, JSON: strings.TrimSpace(buf.String()), JWT: jwt, Doc: doc}, nil
}

func loadTemplate(dir fs.FS) (*offeringTemplate, error) {
	t := &offeringTemplate{credentials: map[string]*template.Template{}}
	var err error
	if t.offering, err = parse(dir, "offering.json"); err != nil {
		return nil, err
	}
	if _, err := fs.Stat(dir, "description.md"); err == nil {
		if t.description, err = parse(dir, "description.md"); err != nil {
			return nil, err
		}
	}
	files, err := fs.Glob(dir, "credentials/*.json")
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		name := strings.TrimSuffix(path.Base(f), ".json")
		if !validName.MatchString(name) {
			return nil, fmt.Errorf("%s: invalid credential name", f)
		}
		if t.credentials[name], err = parse(dir, f); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func parse(dir fs.FS, file string) (*template.Template, error) {
	b, err := fs.ReadFile(dir, file)
	if err != nil {
		return nil, err
	}
	return template.New(file).Option("missingkey=error").Funcs(funcs).Parse(string(b))
}

// renderText executes t.
func renderText(t *template.Template, data templateData) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// render executes t and checks that the result is a JSON object.
func render(t *template.Template, data templateData) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%s: rendered output is not a JSON object", t.Name())
	}
	return buf.Bytes(), nil
}

var funcs = template.FuncMap{
	// json encodes a value as a JSON literal, e.g. "name": {{json .ID}}
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
	// uuid returns a random urn:uuid, e.g. for credential ids
	"uuid": func() (string, error) {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
		return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
	},
	// prop returns the value of the first AAS Property (or MultiLanguageProperty, in English
	// if it has that) with that idShort, searched through all submodels and nested elements;
	// "" if there is none
	"prop": func(dpp map[string]any, idShort string) string {
		v, _ := findProperty(dpp["submodels"], idShort)
		return v
	},
	// env returns an environment variable; only TEMPLATE_* variables are readable, so
	// templates cannot leak the service's secrets
	"env": func(key string) (string, error) {
		if !strings.HasPrefix(key, "TEMPLATE_") {
			return "", fmt.Errorf("env %q: only TEMPLATE_* variables are readable", key)
		}
		return os.Getenv(key), nil
	},
	// default returns def if v is empty
	"default": func(def, v string) string {
		if v == "" {
			return def
		}
		return v
	},
	// slug turns an id into a file-name-like part of a URL: lower case, runs of other
	// characters than a-z and 0-9 as "-", without the URL scheme, e.g. example-com-dpp-x1
	"slug": func(s string) string {
		if i := strings.Index(s, "://"); i >= 0 {
			s = s[i+3:]
		}
		var b strings.Builder
		dash := false
		for _, r := range strings.ToLower(s) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				b.WriteRune(r)
				dash = false
			} else if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		return strings.TrimSuffix(b.String(), "-")
	},
	// trimSuffix removes suffix from s, e.g. {{trimSuffix ".jwt" $url}}
	"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
	// required fails rendering when v is empty, naming what is missing, e.g.
	// {{required "TEMPLATE_TERMS_URL" (env "TEMPLATE_TERMS_URL")}}
	"required": func(what, v string) (string, error) {
		if v == "" {
			return "", fmt.Errorf("%s is required", what)
		}
		return v, nil
	},
	// md makes a value safe for a Markdown table cell or a single line: no line breaks, "|"
	// and Markdown punctuation escaped
	"md": func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		return mdEscaper.Replace(s)
	},
}

var mdEscaper = strings.NewReplacer(`\`, `\\`, "|", `\|`, "*", `\*`, "_", `\_`, "`", "\\`", "[", `\[`, "]", `\]`, "<", "&lt;", ">", "&gt;")

// findProperty walks AAS JSON and returns the string value of the first Property with idShort.
func findProperty(node any, idShort string) (string, bool) {
	switch n := node.(type) {
	case []any:
		for _, c := range n {
			if v, ok := findProperty(c, idShort); ok {
				return v, true
			}
		}
	case map[string]any:
		if n["modelType"] == "Property" && n["idShort"] == idShort {
			v, _ := n["value"].(string)
			return v, true
		}
		if n["modelType"] == "MultiLanguageProperty" && n["idShort"] == idShort {
			return langString(n["value"]), true
		}
		for _, k := range []string{"submodelElements", "value", "statements"} {
			if v, ok := findProperty(n[k], idShort); ok {
				return v, true
			}
		}
	}
	return "", false
}

// langString returns the English text of a MultiLanguageProperty value, else its first text.
func langString(v any) string {
	list, _ := v.([]any)
	first := ""
	for _, x := range list {
		m, _ := x.(map[string]any)
		text, _ := m["text"].(string)
		if lang, _ := m["language"].(string); strings.HasPrefix(lang, "en") {
			return text
		}
		if first == "" {
			first = text
		}
	}
	return first
}
