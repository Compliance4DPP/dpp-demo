package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
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
//	offering.json              PontusxOffering of the Ecosystems Gateway (spp v3, protojson)
//	credentials/<name>.json    Verifiable Credentials (JSON-LD) to sign, zero or more
//
// The offering is complete in itself: file and credential URLs are fixed in the template, as
// the service hosts nothing.
//
// All files are Go text/templates rendered with templateData and the functions in funcs.
type offeringTemplate struct {
	offering    *template.Template
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
}

func loadTemplate(dir fs.FS) (*offeringTemplate, error) {
	t := &offeringTemplate{credentials: map[string]*template.Template{}}
	var err error
	if t.offering, err = parse(dir, "offering.json"); err != nil {
		return nil, err
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

// render executes t and checks that the result is JSON.
func render(t *template.Template, data templateData) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	if !json.Valid(buf.Bytes()) {
		return nil, fmt.Errorf("%s: rendered output is not valid JSON", t.Name())
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
	// prop returns the value of the first AAS Property with that idShort, searched through
	// all submodels and nested elements; "" if there is none
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
}

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
		for _, k := range []string{"submodelElements", "value", "statements"} {
			if v, ok := findProperty(n[k], idShort); ok {
				return v, true
			}
		}
	}
	return "", false
}
