// The browser half of the end-to-end encryption interop check.
//
// Runs web/src/lib/e2e.ts under Node (which provides the same WebCrypto API as
// a browser), so the two implementations of the format can be checked against
// each other rather than each only against itself.
//
//   node interop.mjs encrypt <key> <name> <in> <out>
//   node interop.mjs decrypt <key> <in> <out>      # prints the metadata JSON
//
// The module under test is passed in already stripped of its TypeScript types;
// see run.sh.

import { readFile, writeFile } from "node:fs/promises";
import process from "node:process";

const [, , command, ...args] = process.argv;

const modulePath = process.env.E2E_MODULE;
if (!modulePath) {
  console.error("E2E_MODULE must point at the compiled e2e module");
  process.exit(1);
}

const e2e = await import(modulePath);

try {
  switch (command) {
    case "encrypt": {
      const [key, name, input, output] = args;
      const data = await readFile(input);
      const blob = await e2e.encrypt(new Blob([data]), e2e.decodeKey(key), {
        name,
        type: "application/octet-stream",
      });
      await writeFile(output, Buffer.from(await blob.arrayBuffer()));
      break;
    }

    case "decrypt": {
      const [key, input, output] = args;
      const data = await readFile(input);
      const { meta, blob } = await e2e.decrypt(new Uint8Array(data), e2e.decodeKey(key));
      await writeFile(output, Buffer.from(await blob.arrayBuffer()));
      console.log(JSON.stringify(meta));
      break;
    }

    case "genkey": {
      console.log(e2e.encodeKey(e2e.generateKey()));
      break;
    }

    default:
      console.error(`unknown command ${command}`);
      process.exit(1);
  }
} catch (err) {
  console.error("error:", err instanceof Error ? err.message : err);
  process.exit(1);
}
