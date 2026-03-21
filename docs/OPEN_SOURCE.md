# Open Source And Licensing

WolfBBS is released under the MIT License.

- Canonical license file: [../LICENSE](../LICENSE)
- Contribution policy: [../CONTRIBUTING.md](../CONTRIBUTING.md)
- Security policy: [../SECURITY.md](../SECURITY.md)

## What is covered

Unless a file says otherwise, the source code, original documentation, and
original project assets included in this repository are provided under the MIT
License.

## Third-party software

WolfBBS depends on third-party libraries, tools, and base images. Those
dependencies keep their own licenses; they are not relicensed by WolfBBS.

That applies in particular to:

- Go module dependencies from `go.mod`
- npm/browser test dependencies under `e2e/web`
- container base images and operating-system packages used in Docker builds

If you redistribute WolfBBS, keep the WolfBBS MIT license notice intact and
review third-party license obligations for the dependencies you ship with it.

## Distribution bundles

Release bundles should include:

- the root `LICENSE`
- the root `CONTRIBUTING.md`
- this document

This keeps GitHub, packaged distributions, and downstream operators aligned on
the same licensing story.
