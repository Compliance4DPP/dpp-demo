const $ = (id) => document.getElementById(id);

document.querySelectorAll("button[data-s]").forEach((b) => {
  b.onclick = async () => {
    $("input").value = await (await fetch(`samples/${b.dataset.s}.json`)).text();
    check();
  };
});
$("check").onclick = check;
$("tech").onchange = () => {
  if ($("input").value.trim()) check();
};

async function check() {
  $("vcbox").hidden = true;
  $("results").innerHTML = "";
  $("raw").textContent = "";
  const fail = (title, detail) => {
    $("verdict").innerHTML = `<span class="FAIL">${title}</span><span class="hint muted">${esc(detail)}</span>`;
  };

  if (!$("input").value.trim()) return fail("Nothing to check", "Paste an AAS file or pick a sample first.");
  let input;
  try {
    input = JSON.parse($("input").value);
  } catch (e) {
    return fail("Not valid JSON", e.message);
  }

  $("verdict").innerHTML = '<span class="hint muted">Checking\u2026</span>';
  let body;
  try {
    const res = await fetch(`/v1/data/dpp/${$("tech").checked ? "report_tech" : "report"}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ input }),
    });
    body = await res.json();
  } catch (e) {
    return fail("Check failed", "The policy service could not be reached (" + e.message + ").");
  }
  $("raw").textContent = JSON.stringify(body, null, 2);

  const r = body.result;
  if (!r) return fail("No result", "OPA returned no result for this input.");
  $("verdict").innerHTML = r.allow
    ? '<span class="PASS">Compliant</span>'
    : `<span class="FAIL">Not compliant</span> (${r.failed} failed)`;
  showResults(r.results, r.groups);
  showVc(r);
}

const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]);

const order = { FAIL: 0, SKIP: 1, PASS: 2 };
const n = (rows, s) => rows.filter((x) => x.status === s).length;

function showResults(results, groups) {
  const by = groups.map((name) => [name, results.filter((x) => x.group === name)]);
  // zero counts stay grey
  const count = (s, label) =>
    `<span class="count ${n(results, s) ? s : "SKIP"}"><b>${n(results, s)}</b> ${label}</span>`;
  $("results").innerHTML =
    `<p class="counts">${count("PASS", "passed")}${count("FAIL", "failed")}${count("SKIP", "skipped")}</p>` +
    by
      .filter(([, rows]) => rows.length)
      .map(([name, rows]) => {
        const fail = n(rows, "FAIL"),
          skip = n(rows, "SKIP");
        rows.sort((a, b) => order[a.status] - order[b.status]);
        return (
          `<details class="grp"${fail ? " open" : ""}><summary><b>${esc(name)}</b>` +
          `<span class="${fail ? "FAIL" : "PASS"}">${fail ? fail + " failed" : "all passed"}</span>` +
          `<span class="muted">${rows.length} check${rows.length > 1 ? "s" : ""}${skip ? ", " + skip + " skipped" : ""}</span></summary><table>` +
          rows
            .map(
              (x) =>
                `<tr><td class="${x.status}">${x.status}</td><td>${esc(x.message)}</td><td><code>${esc(x.check)}</code></td></tr>`,
            )
            .join("") +
          "</table></details>"
        );
      })
      .join("");
}

function showVc(r) {
  $("vcbox").hidden = !r.vc;
  if (!r.vc) return;
  const cs = r.vc.credentialSubject,
    c = cs["gx:compliantCredentials"][0],
    signed = !!r.vc_jwt;
  $("vcinfo").innerHTML = [
    [
      "Signature",
      signed
        ? "signed (PS256), verified below against the issuer's public key"
        : r.vc_error || "unsigned: no signing key configured",
    ],
    ["Subject", c.id],
    ["Rules", cs["gx:rulesVersion"] + ", " + cs["gx:validatedCriteria"].length + " checks passed"],
    ["Input digest", c.digestSRI],
    ["Valid", r.vc.validFrom + " to " + r.vc.validUntil],
  ]
    .map(([k, v]) => `<dt>${k}</dt><dd>${esc(v)}</dd>`)
    .join("");
  $("vctitle").textContent = signed
    ? "Credential issued by "
    : r.vc_error
      ? "Credential NOT signed: "
      : "Unsigned credential for ";
  $("vcbox").className = signed ? "" : "bad";
  $("vcissuer").textContent = r.vc.issuer;
  $("jwt").value = r.vc_jwt || "";
  $("vc").textContent = JSON.stringify([r.vc], null, 2);
  $("vcverify").textContent = "";
  for (const id of ["verify", "copy", "jwt"]) $(id).hidden = !signed;
  document.querySelector('label[for="jwt"]').hidden = !signed;
  if (signed) $("verify").onclick(); // verify right away, so a wrong key is visible without a click
}

$("copy").onclick = () => navigator.clipboard.writeText($("jwt").value);

const b64u = (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));

// did.json comes through nginx: the issuer sends no CORS header
$("verify").onclick = async () => {
  const out = (ok, msg) => {
    $("vcverify").innerHTML =
      `<span class="${ok ? "PASS" : "FAIL"}">${ok ? "Signature valid" : "Not valid"}</span>: ${esc(msg)} <span class="muted">(checked ${new Date().toLocaleTimeString()})</span>`;
  };
  $("vcverify").textContent = "Checking\u2026";
  await new Promise((r) => setTimeout(r, 300)); // long enough to see that a click re-checks
  try {
    const [h, p, sig] = $("jwt").value.trim().split(".");
    const { kid } = JSON.parse(new TextDecoder().decode(b64u(h)));
    const doc = await (await fetch("did.json")).json();
    const vm = doc.verificationMethod.find((m) => m.id === kid);
    if (!vm) return out(false, `key ${kid} is not in the issuer's DID document`);
    if (vm.revoked) return out(false, `key ${kid} was revoked on ${vm.revoked}`);
    const { kty, n, e } = vm.publicKeyJwk;
    const key = await crypto.subtle.importKey("jwk", { kty, n, e }, { name: "RSA-PSS", hash: "SHA-256" }, false, [
      "verify",
    ]);
    const ok = await crypto.subtle.verify(
      { name: "RSA-PSS", saltLength: 32 },
      key,
      b64u(sig),
      new TextEncoder().encode(`${h}.${p}`),
    );
    out(
      ok,
      ok
        ? `signed by ${kid}, public key fetched from the issuer's did.json`
        : `signature does not match the content (key ${kid})`,
    );
  } catch (err) {
    out(false, "not a readable JWT (" + err.message + ")");
  }
};
