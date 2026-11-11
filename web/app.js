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
}

const esc = (s) => String(s).replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" }[c]));
