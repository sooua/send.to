# Third-Party Licenses

This project bundles third-party code. The MIT license requires these
attributions to be distributed alongside the software.

---

## `internal/clamd` — ClamAV protocol client

A modified, lightly cleaned-up copy of a public-domain-era ClamAV TCP
protocol client. Original upstream is no longer maintained; the code in
`internal/clamd/` has been reformatted, had ignored errors fixed, and
naming conventions updated to idiomatic Go.

```
The MIT License (MIT)

Copyright (c) 2013 the original authors

Permission is hereby granted, free of charge, to any person obtaining a
copy of this software and associated documentation files (the
"Software"), to deal in the Software without restriction, including
without limitation the rights to use, copy, modify, merge, publish,
distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to
the following conditions:

The above copyright notice and this permission notice shall be included
in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

---

## Go module dependencies

Transitive Go module licenses are declared in each module's own source.
Run `go mod download && find $(go env GOMODCACHE) -name LICENSE` for a
full listing, or see `go.sum`.

## Frontend (web/) npm dependencies

See `web/package-lock.json` for the full dependency tree. Run
`cd web && npx license-checker` or similar tooling to extract per-package
license metadata.
