import { test } from "node:test";
import assert from "node:assert/strict";
import { verifyJwt } from "../web/verify.js";

const kid = "did:web:example.com#k1";
const now = new Date("2026-06-01T00:00:00Z");
const enc = (o) => Buffer.from(JSON.stringify(o)).toString("base64url");

const { privateKey, publicKey } = await crypto.subtle.generateKey(
  { name: "RSA-PSS", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
  true,
  ["sign", "verify"],
);
const publicKeyJwk = await crypto.subtle.exportKey("jwk", publicKey);

async function sign(payload, header = { alg: "PS256", typ: "vc+jwt", kid }) {
  const input = `${enc(header)}.${enc(payload)}`;
  const sig = await crypto.subtle.sign(
    { name: "RSA-PSS", saltLength: 32 },
    privateKey,
    new TextEncoder().encode(input),
  );
  return `${input}.${Buffer.from(sig).toString("base64url")}`;
}

const doc = (vm = {}) => ({ verificationMethod: [{ id: kid, expires: "2027-01-01T00:00:00Z", publicKeyJwk, ...vm }] });
const vc = { validUntil: "2026-09-01T00:00:00Z" };

test("valid credential passes", async () => {
  assert.deepEqual(await verifyJwt(await sign(vc), doc(), now), [
    true,
    `signed by ${kid}, public key fetched from the issuer's did.json`,
  ]);
});

test("surrounding whitespace is ignored", async () => {
  assert.equal((await verifyJwt(` ${await sign(vc)}\n`, doc(), now))[0], true);
});

test("tampered payload fails the signature", async () => {
  const [h, , s] = (await sign(vc)).split(".");
  const [ok, msg] = await verifyJwt(`${h}.${enc({ validUntil: "2099-01-01T00:00:00Z" })}.${s}`, doc(), now);
  assert.equal(ok, false);
  assert.match(msg, /signature does not match/);
});

test("signature from another key fails", async () => {
  const other = await crypto.subtle.generateKey(
    { name: "RSA-PSS", modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: "SHA-256" },
    true,
    ["sign", "verify"],
  );
  const jwk = await crypto.subtle.exportKey("jwk", other.publicKey);
  const [ok, msg] = await verifyJwt(await sign(vc), doc({ publicKeyJwk: jwk }), now);
  assert.equal(ok, false);
  assert.match(msg, /signature does not match/);
});

test("unknown kid fails", async () => {
  const [ok, msg] = await verifyJwt(await sign(vc, { alg: "PS256", kid: kid + "x" }), doc(), now);
  assert.equal(ok, false);
  assert.match(msg, /is not in the issuer's DID document/);
});

test("revoked key fails even with a good signature", async () => {
  const [ok, msg] = await verifyJwt(await sign(vc), doc({ revoked: "2026-05-01T00:00:00Z" }), now);
  assert.equal(ok, false);
  assert.match(msg, /was revoked on 2026-05-01/);
});

test("expired key fails", async () => {
  const [ok, msg] = await verifyJwt(await sign(vc), doc({ expires: "2026-05-31T23:59:59Z" }), now);
  assert.equal(ok, false);
  assert.match(msg, /expired on 2026-05-31/);
});

test("expired credential fails", async () => {
  const [ok, msg] = await verifyJwt(await sign({ validUntil: "2026-05-31T00:00:00Z" }), doc(), now);
  assert.equal(ok, false);
  assert.match(msg, /the signature matches, but the credential expired/);
});

for (const jwt of ["", "abc", "a.b.c", "!!!.e30.e30"]) {
  test(`unreadable JWT ${JSON.stringify(jwt)} fails without throwing`, async () => {
    const [ok, msg] = await verifyJwt(jwt, doc(), now);
    assert.equal(ok, false);
    assert.match(msg, /not a readable JWT/);
  });
}
