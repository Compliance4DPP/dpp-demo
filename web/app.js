const $ = (id) => document.getElementById(id);

document.querySelectorAll("button[data-s]").forEach((b) => {
  b.onclick = async () => {
    $("input").value = await (await fetch(`samples/${b.dataset.s}.json`)).text();
    check();
  };
});
$("check").onclick = check;
$("tech").onchange = () => { if ($("input").value.trim()) check(); };

async function check() {
  let input;
  try { input = JSON.parse($("input").value); }
  catch (e) { $("verdict").textContent = "Not valid JSON: " + e.message; return; }

  // OPA Data API: POST /v1/data/<package>/<rule> with {"input": ...}; report_tech = profile with technical data
  const res = await fetch(`/v1/data/dpp/${$("tech").checked ? "report_tech" : "report"}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ input }),
  });
  const body = await res.json();
  $("raw").textContent = JSON.stringify(body, null, 2);

  const r = body.result;
  if (!r) { $("verdict").textContent = "OPA returned no result"; return; }
  $("verdict").innerHTML = r.allow
    ? '<span class="PASS">Compliant</span>'
    : `<span class="FAIL">Not compliant</span> (${r.failed} failed)`;
  $("results").innerHTML = "<tr><th>Check</th><th>Status</th><th>Message</th></tr>" +
    r.results.map((x) =>
      `<tr><td>${esc(x.check)}</td><td class="${x.status}">${x.status}</td><td>${esc(x.message)}</td></tr>`
    ).join("");
  showVc(r);
}

const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));

function showVc(r) {
  $("vcbox").hidden = !r.vc;
  if (!r.vc) return;
  const cs = r.vc.credentialSubject, c = cs["gx:compliantCredentials"][0], signed = !!r.vc_jwt;
  $("vcinfo").innerHTML = [
    ["Signature", signed ? "signed (PS256), verified below against the issuer's public key" : r.vc_error || "unsigned: no signing key configured"],
    ["Subject", c.id],
    ["Rules", cs["gx:rulesVersion"] + ", " + cs["gx:validatedCriteria"].length + " checks passed"],
    ["Input digest", c.digestSRI],
    ["Valid", r.vc.validFrom + " to " + r.vc.validUntil],
  ].map(([k, v]) => `<dt>${k}</dt><dd>${esc(v)}</dd>`).join("");
  $("vctitle").textContent = signed ? "Credential issued by " : r.vc_error ? "Credential NOT signed: " : "Unsigned credential for ";
  $("vcbox").className = signed ? "" : "bad";
  $("vcissuer").textContent = r.vc.issuer;
  $("jwt").value = r.vc_jwt || "";
  $("vc").textContent = JSON.stringify([r.vc], null, 2);
  $("vcverify").textContent = "";
  for (const id of ["verify", "copy", "jwt"]) $(id).hidden = !signed;
  document.querySelector('label[for="jwt"]').hidden = !signed;
  if (signed) $("verify").onclick();  // verify right away, so a wrong key is visible without a click
}

$("copy").onclick = () => navigator.clipboard.writeText($("jwt").value);

const b64u = (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));

// Verifies the JWT in the browser against the issuer's published DID document (did.json is proxied by nginx: no CORS there).
$("verify").onclick = async () => {
  const out = (ok, msg) => { $("vcverify").innerHTML = `<span class="${ok ? "PASS" : "FAIL"}">${ok ? "Signature valid" : "Not valid"}</span>: ${esc(msg)}`; };
  try {
    const [h, p, sig] = $("jwt").value.trim().split(".");
    const { kid } = JSON.parse(new TextDecoder().decode(b64u(h)));
    const doc = await (await fetch("did.json")).json();
    const vm = doc.verificationMethod.find((m) => m.id === kid);
    if (!vm) return out(false, `key ${kid} is not in the issuer's DID document`);
    if (vm.revoked) return out(false, `key ${kid} was revoked on ${vm.revoked}`);
    const { kty, n, e } = vm.publicKeyJwk;
    const key = await crypto.subtle.importKey("jwk", { kty, n, e }, { name: "RSA-PSS", hash: "SHA-256" }, false, ["verify"]);
    const ok = await crypto.subtle.verify({ name: "RSA-PSS", saltLength: 32 }, key, b64u(sig), new TextEncoder().encode(`${h}.${p}`));
    out(ok, ok ? `signed by ${kid}, public key fetched from the issuer's did.json` : `signature does not match the content (key ${kid})`);
  } catch (err) { out(false, "not a readable JWT (" + err.message + ")"); }
};
