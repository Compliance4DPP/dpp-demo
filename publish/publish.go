package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"regexp"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"dpp-demo/publish/gen/gatewayv3"
	"dpp-demo/publish/gen/publishv1"
	"dpp-demo/publish/gen/signerpb"
)

// validName is what DPP, template and credential names may look like (no paths).
var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type publisher struct {
	publishv1.UnimplementedPublisherServer

	dppDir, templateDir string
	issuer              string
	signerAuth          string
	timeout             time.Duration

	signer  signerpb.SignerClient
	gateway gatewayv3.EcosystemsgatewayClient
}

func (p *publisher) Publish(ctx context.Context, req *publishv1.PublishRequest) (*publishv1.PublishResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	if !validName.MatchString(req.GetTemplate()) {
		return nil, status.Error(codes.InvalidArgument, "template: missing or invalid name")
	}
	tmpl, err := loadTemplate(os.DirFS(p.templateDir + "/" + req.GetTemplate()))
	if err != nil {
		return nil, fileError("template "+req.GetTemplate(), err)
	}

	raw, err := p.loadDPP(req)
	if err != nil {
		return nil, err
	}
	var dpp map[string]any
	if err := json.Unmarshal(raw, &dpp); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "dpp: not a JSON object: %v", err)
	}

	id := dppID(dpp, raw)
	log := slog.With("dpp", id, "template", req.GetTemplate())
	data := templateData{DPP: dpp, ID: id, Issuer: p.issuer, Now: time.Now().UTC().Format(time.RFC3339)}

	// Render everything first, so a broken template fails before anything is signed or published.
	creds := map[string][]byte{}
	for _, name := range tmpl.credentialNames() {
		if creds[name], err = render(tmpl.credentials[name], data); err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, "template: %v", err)
		}
	}
	offeringJSON, err := render(tmpl.offering, data)
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "template: %v", err)
	}
	offering := &gatewayv3.PontusxOffering{}
	if err := protojson.Unmarshal(offeringJSON, offering); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "template: offering.json is no PontusxOffering: %v", err)
	}

	signed := map[string]string{}
	for _, name := range tmpl.credentialNames() {
		if signed[name], err = p.sign(ctx, creds[name]); err != nil {
			return nil, upstream("signer: credential "+name, err)
		}
		log.Info("credential signed", "credential", name)
	}

	res, err := p.gateway.CreateOffering(ctx, &gatewayv3.CreateOfferingRequest{
		Offerings: []*gatewayv3.CreateOfferingRequest_Offering{{
			EcosystemOffering: &gatewayv3.CreateOfferingRequest_Offering_PontusxOffering{PontusxOffering: offering},
		}},
	})
	if err != nil {
		return nil, upstream("gateway", err)
	}
	log.Info("offering published", "ids", res.GetId())

	return &publishv1.PublishResponse{OfferingIds: res.GetId(), Credentials: signed}, nil
}

func (p *publisher) loadDPP(req *publishv1.PublishRequest) ([]byte, error) {
	switch d := req.GetDpp().(type) {
	case *publishv1.PublishRequest_DppJson:
		return d.DppJson, nil
	case *publishv1.PublishRequest_DppName:
		if !validName.MatchString(d.DppName) {
			return nil, status.Error(codes.InvalidArgument, "dpp_name: invalid name")
		}
		b, err := os.ReadFile(p.dppDir + "/" + d.DppName + ".json")
		if err != nil {
			return nil, fileError("dpp "+d.DppName, err)
		}
		return b, nil
	default:
		return nil, status.Error(codes.InvalidArgument, "dpp: set dpp_name or dpp_json")
	}
}

// sign has the signer sign a JSON-LD credential and returns it as compact JWS.
func (p *publisher) sign(ctx context.Context, credential []byte) (string, error) {
	s := &structpb.Struct{}
	if err := protojson.Unmarshal(credential, s); err != nil {
		return "", status.Errorf(codes.FailedPrecondition, "not a JSON object: %v", err)
	}
	if p.signerAuth != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", p.signerAuth)
	}
	res, err := p.signer.SignVC(ctx, &signerpb.VC{Vc: &signerpb.VC_Jsonld{Jsonld: s}})
	if err != nil {
		return "", err
	}
	jwt := res.GetJwtld()
	if jwt == "" {
		return "", status.Error(codes.Internal, "signer returned no JWT")
	}
	return jwt, nil
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

// upstream wraps an error of a dependency, keeping its gRPC code.
func upstream(step string, err error) error {
	if s, ok := status.FromError(err); ok {
		return status.Errorf(s.Code(), "%s: %s", step, s.Message())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Errorf(codes.DeadlineExceeded, "%s: %v", step, err)
	}
	return status.Errorf(codes.Unavailable, "%s: %v", step, err)
}

func fileError(what string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return status.Errorf(codes.NotFound, "%s: %v", what, err)
	}
	return status.Errorf(codes.FailedPrecondition, "%s: %v", what, err)
}
