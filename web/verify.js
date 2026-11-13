// Checks a vc+jwt against the issuer's DID document. Returns [ok, message]; never throws.
const b64u = (s) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/")), (c) => c.charCodeAt(0));
const json = (s) => JSON.parse(new TextDecoder().decode(b64u(s)));

export async function verifyJwt(jwt, doc, now = new Date()) {
  try {
    const [h, p, sig] = jwt.trim().split(".");
    const { kid } = json(h);
    const vm = doc.verificationMethod.find((m) => m.id === kid);
    if (!vm) return [false, `key ${kid} is not in the issuer's DID document`];
    if (vm.revoked) return [false, `key ${kid} was revoked on ${vm.revoked}`];
    if (new Date(vm.expires) < now) return [false, `key ${kid} expired on ${vm.expires}`];
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
    if (!ok) return [false, `signature does not match the content (key ${kid})`];
    const { validUntil } = json(p);
    if (new Date(validUntil) < now)
      return [false, `the signature matches, but the credential expired on ${validUntil}`];
    return [true, `signed by ${kid}, public key fetched from the issuer's did.json`];
  } catch (err) {
    return [false, "not a readable JWT (" + err.message + ")"];
  }
}
