# Message Network Baseline

WolfBBS now ships a scanner/tosser-friendly packet spool baseline in `internal/network`.

## Formats

- `ftn`
- `bso`
- `qwk`
- `netmail`

## Spool layout

Default root: `.wolfbbs/network` (override with `WOLFBBS_NET_SPOOL_DIR`).

- `outbound/<format>/*.json`
- `inbound/<format>/*.json`
- `processed/<format>/*.json`

## Sysop CLI

```bash
oputil network status
oputil network export --format qwk --board 1 --out /tmp/board1.qwk.json
oputil network import --in /tmp/board1.qwk.json --author 1
oputil network queue-netmail --from 1 --to caller --subject "hello" --body "test"
oputil network import-queue --author 1
```

## Notes

- This is a baseline transport shape for FTN/BSO/QWK/netmail integration.
- External tosser or network bridge services can now target the spool contract.
- Secure-by-default: no automatic external network transport is enabled by this module.
