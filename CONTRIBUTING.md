# Contributing to WolfBBS

Thanks for contributing.

## Ground rules

- Keep changes focused and explain both what changed and why.
- Run the relevant validation for the surfaces you touched.
- Update docs when public behavior changes.

## Validation baseline

- `go test ./...`
- `scripts/verify.sh --fast`
- `scripts/verify.sh --smoke` or a justified skip
- `scripts/run-e2e.sh` or a justified skip for affected UI changes

## Licensing

By submitting a contribution to WolfBBS, you agree that your contribution is
licensed under the MIT License in the root [LICENSE](LICENSE) file, unless a
different arrangement is explicitly agreed in writing first.
