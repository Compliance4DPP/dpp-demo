import { verifyJwt } from "./verify.js";

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

let runs = 0;
// the DPP and compliance credential of the last passing check, for publishing
let checked = null;
// where published offerings are shown
const PORTAL = "https://portal.compliance4dpp.eu";

async function check() {
  const run = ++runs;
  checked = null;
  $("vcbox").hidden = true;
  $("results").innerHTML = "";
  $("raw").textContent = "";
  const fail = (title, detail) => {
    if (run !== runs) return;
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
    if (!res.ok) return fail("Check failed", "The policy service answered HTTP " + res.status + ".");
    body = await res.json();
  } catch (e) {
    return fail("Check failed", "The policy service could not be reached (" + e.message + ").");
  }
  if (run !== runs) return;
  $("raw").textContent = JSON.stringify(body, null, 2);

  const r = body.result;
  if (!r) return fail("No result", "OPA returned no result for this input.");
  $("verdict").innerHTML = r.allow
    ? '<span class="PASS">Compliant</span>'
    : `<span class="FAIL">Not compliant</span> (${r.failed} failed)`;
  showResults(r.results, r.groups);
  showVc(r);
  // only a passing check (which issued a credential) can be published
  checked = r.allow && r.vc ? { dpp: input, credential: r.vc_jwt || r.vc } : null;
  $("pub").hidden = !checked;
  $("publish").disabled = false;
  $("pubresult").textContent = "";
}

// Sends the checked DPP and its compliance credential (the JWT if signed) to the publish
// service, which nginx forwards to the dataswitch container of the publishing stack.
$("publish").onclick = async () => {
  if (!checked) return;
  const sent = checked;
  const out = (html) => {
    if (checked === sent) $("pubresult").innerHTML = html;
  };
  $("publish").disabled = true;
  out(
    '<span class="muted">Publishing… the gateway waits until Pontus-X has indexed the offering, which can take a few minutes.</span>',
  );
  try {
    const res = await fetch("/publish", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ template: "default", dpp: sent.dpp, compliance_credential: sent.credential }),
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) {
      const why =
        res.status === 502 && !body.error
          ? "the publish service is not running (start it with docker compose --profile publish up)"
          : body.error || "HTTP " + res.status;
      return out(`<span class="FAIL">Not published</span>: ${esc(why)}`);
    }
    // each offering's DID, linked to its page in the COMPLIANCE4DPP portal (only well-formed DIDs get a link)
    const links = (body.offering_ids || [])
      .map((id) =>
        /^did:[a-z0-9]+:[A-Za-z0-9._-]+$/.test(id)
          ? `<code>${esc(id)}</code> <a href="${PORTAL}/asset/${id}" target="_blank" rel="noopener">open in the COMPLIANCE4DPP portal</a>`
          : `<code>${esc(id)}</code>`,
      )
      .join("<br />");
    out(`<p><span class="PASS">Published</span>: ${links || "no offering id returned"}</p>`);
  } catch (e) {
    out(`<span class="FAIL">Not published</span>: the publish service could not be reached (${esc(e.message)})`);
  } finally {
    if (checked === sent) $("publish").disabled = false;
  }
};

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

// did.json comes through nginx: the issuer sends no CORS header
$("verify").onclick = async () => {
  const out = (ok, msg) => {
    $("vcverify").innerHTML =
      `<span class="${ok ? "PASS" : "FAIL"}">${ok ? "Signature valid" : "Not valid"}</span>: ${esc(msg)} <span class="muted">(checked ${new Date().toLocaleTimeString()})</span>`;
  };
  $("vcverify").textContent = "Checking\u2026";
  await new Promise((r) => setTimeout(r, 300)); // long enough to see that a click re-checks
  let doc;
  try {
    const res = await fetch("did.json");
    if (!res.ok) return out(false, `the issuer's did.json could not be fetched (HTTP ${res.status})`);
    doc = await res.json();
  } catch (e) {
    return out(false, "the issuer's did.json could not be fetched (" + e.message + ")");
  }
  out(...(await verifyJwt($("jwt").value, doc)));
};
