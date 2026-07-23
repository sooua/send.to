// End-to-end encryption in the browser.
//
// Byte-for-byte compatible with the Go implementation in client/e2e.go — see
// that file for the format and the reasoning. In short:
//
//   "STE1"            4 bytes  magic and version
//   nonce prefix      7 bytes  random
//   metadata length   2 bytes  big-endian, ciphertext incl. tag
//   chunk 0         variable   AES-256-GCM(metadata JSON)
//   chunk 1..n                 AES-256-GCM(64 KiB of plaintext)
//
//   nonce = prefix || counter(uint32 BE) || (1 on the last chunk else 0)
//
// The key lives in the URL fragment, which browsers never put on the wire, so
// the server stores ciphertext it cannot read.

const MAGIC = "STE1";
const KEY_SIZE = 32;
const NONCE_PREFIX_SIZE = 7;
const META_LEN_SIZE = 2;
const CHUNK_SIZE = 64 * 1024;
const TAG_SIZE = 16;

const HEADER_SIZE = MAGIC.length + NONCE_PREFIX_SIZE + META_LEN_SIZE;
const CHUNK_CIPHER_SIZE = CHUNK_SIZE + TAG_SIZE;

export interface E2EMetadata {
  name: string;
  type?: string;
}

export class DecryptError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "DecryptError";
  }
}

/** A fresh 256-bit content key. */
export function generateKey(): Uint8Array {
  return crypto.getRandomValues(new Uint8Array(KEY_SIZE));
}

/** Unpadded base64url, matching Go's base64.RawURLEncoding. */
export function encodeKey(key: Uint8Array): string {
  let binary = "";
  for (const byte of key) binary += String.fromCharCode(byte);
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function decodeKey(encoded: string): Uint8Array {
  const normalized = encoded.trim().replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized + "=".repeat((4 - (normalized.length % 4)) % 4);

  let binary: string;
  try {
    binary = atob(padded);
  } catch {
    throw new DecryptError("malformed key");
  }

  if (binary.length !== KEY_SIZE) {
    throw new DecryptError(`malformed key: got ${binary.length} bytes, want ${KEY_SIZE}`);
  }

  const key = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) key[i] = binary.charCodeAt(i);
  return key;
}

/**
 * Read the key out of a share URL's fragment. Accepts `#k=<key>` and a bare
 * `#<key>`; returns null when there is no fragment.
 */
export function keyFromFragment(fragment: string): Uint8Array | null {
  const raw = fragment.replace(/^#/, "").replace(/^k=/, "");
  if (!raw) return null;
  return decodeKey(raw);
}

function nonce(prefix: Uint8Array, counter: number, final: boolean): Uint8Array {
  const iv = new Uint8Array(12);
  iv.set(prefix, 0);
  new DataView(iv.buffer).setUint32(NONCE_PREFIX_SIZE, counter, false); // big-endian
  iv[11] = final ? 1 : 0;
  return iv;
}

async function importKey(key: Uint8Array): Promise<CryptoKey> {
  return crypto.subtle.importKey("raw", key as BufferSource, "AES-GCM", false, [
    "encrypt",
    "decrypt",
  ]);
}

async function seal(
  cryptoKey: CryptoKey,
  iv: Uint8Array,
  plaintext: Uint8Array
): Promise<Uint8Array> {
  const sealed = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: iv as BufferSource, tagLength: TAG_SIZE * 8 },
    cryptoKey,
    plaintext as BufferSource
  );
  return new Uint8Array(sealed);
}

async function open(
  cryptoKey: CryptoKey,
  iv: Uint8Array,
  ciphertext: Uint8Array
): Promise<Uint8Array> {
  try {
    const opened = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv: iv as BufferSource, tagLength: TAG_SIZE * 8 },
      cryptoKey,
      ciphertext as BufferSource
    );
    return new Uint8Array(opened);
  } catch {
    throw new DecryptError("wrong key, or the file was modified or truncated");
  }
}

/**
 * Encrypt a blob. The result is assembled as a Blob rather than one buffer, so
 * the browser can spill it to disk instead of holding the whole file in memory.
 */
export async function encrypt(
  data: Blob,
  key: Uint8Array,
  meta: E2EMetadata,
  onProgress?: (done: number, total: number) => void
): Promise<Blob> {
  const cryptoKey = await importKey(key);
  const prefix = crypto.getRandomValues(new Uint8Array(NONCE_PREFIX_SIZE));

  const metaJSON = new TextEncoder().encode(JSON.stringify(meta));
  const sealedMeta = await seal(cryptoKey, nonce(prefix, 0, false), metaJSON);

  if (sealedMeta.length > 0xffff) throw new Error("metadata is too large");

  const header = new Uint8Array(HEADER_SIZE);
  header.set(new TextEncoder().encode(MAGIC), 0);
  header.set(prefix, MAGIC.length);
  new DataView(header.buffer).setUint16(MAGIC.length + NONCE_PREFIX_SIZE, sealedMeta.length, false);

  const parts: BlobPart[] = [header as BlobPart, sealedMeta as BlobPart];

  // A zero-length payload still produces one empty, final chunk, so that the
  // stream always ends with the final marker.
  const chunkCount = Math.max(1, Math.ceil(data.size / CHUNK_SIZE));

  for (let i = 0; i < chunkCount; i++) {
    const start = i * CHUNK_SIZE;
    const slice = new Uint8Array(await data.slice(start, start + CHUNK_SIZE).arrayBuffer());
    const final = i === chunkCount - 1;

    parts.push((await seal(cryptoKey, nonce(prefix, i + 1, final), slice)) as BlobPart);
    onProgress?.(Math.min(start + CHUNK_SIZE, data.size), data.size);
  }

  return new Blob(parts, { type: "application/octet-stream" });
}

/** The exact ciphertext length for a plaintext of `size` bytes. */
export function encryptedSize(size: number, meta: E2EMetadata): number {
  const metaLen = new TextEncoder().encode(JSON.stringify(meta)).length;
  const chunkCount = Math.max(1, Math.ceil(size / CHUNK_SIZE));
  return HEADER_SIZE + metaLen + TAG_SIZE + size + chunkCount * TAG_SIZE;
}

/**
 * Decrypt a whole ciphertext. Throws DecryptError on a wrong key, a modified
 * payload, or a truncated stream — all three are indistinguishable by design.
 */
export async function decrypt(
  ciphertext: Uint8Array,
  key: Uint8Array,
  onProgress?: (done: number, total: number) => void
): Promise<{ meta: E2EMetadata; blob: Blob }> {
  if (ciphertext.length < HEADER_SIZE) {
    throw new DecryptError("not an end-to-end encrypted file");
  }

  const magic = new TextDecoder().decode(ciphertext.subarray(0, MAGIC.length));
  if (magic !== MAGIC) {
    throw new DecryptError("not an end-to-end encrypted file (or a newer format)");
  }

  const cryptoKey = await importKey(key);
  const prefix = ciphertext.subarray(MAGIC.length, MAGIC.length + NONCE_PREFIX_SIZE);

  const view = new DataView(ciphertext.buffer, ciphertext.byteOffset, ciphertext.byteLength);
  const metaLen = view.getUint16(MAGIC.length + NONCE_PREFIX_SIZE, false);

  const metaEnd = HEADER_SIZE + metaLen;
  if (metaEnd > ciphertext.length) throw new DecryptError("truncated file");

  const metaJSON = await open(
    cryptoKey,
    nonce(prefix, 0, false),
    ciphertext.subarray(HEADER_SIZE, metaEnd)
  );

  let meta: E2EMetadata;
  try {
    meta = JSON.parse(new TextDecoder().decode(metaJSON));
  } catch {
    throw new DecryptError("wrong key, or the file was modified or truncated");
  }

  // Chunk layout is fully determined by the remaining length, so the final
  // chunk — and therefore the final marker — is known without a lookahead.
  const remaining = ciphertext.length - metaEnd;
  const fullChunks = Math.floor(remaining / CHUNK_CIPHER_SIZE);
  const rest = remaining % CHUNK_CIPHER_SIZE;

  if (rest !== 0 && rest < TAG_SIZE) throw new DecryptError("truncated file");

  const chunkCount = rest > 0 ? fullChunks + 1 : fullChunks;
  if (chunkCount === 0) throw new DecryptError("truncated file");

  const parts: BlobPart[] = [];
  let offset = metaEnd;

  for (let i = 0; i < chunkCount; i++) {
    const size = i === chunkCount - 1 && rest > 0 ? rest : CHUNK_CIPHER_SIZE;
    const final = i === chunkCount - 1;

    const plain = await open(
      cryptoKey,
      nonce(prefix, i + 1, final),
      ciphertext.subarray(offset, offset + size)
    );

    parts.push(plain as BlobPart);
    offset += size;
    onProgress?.(offset - metaEnd, remaining);
  }

  return { meta, blob: new Blob(parts, { type: meta.type || "application/octet-stream" }) };
}
