# Extension SDK Starter

WolfBBS ships a starter extension pack generator for operators who want a safe baseline before adding custom plugin logic.

## Quick start

1. Open `/admin/plugins` as a sysop/admin.
2. In **Starter SDK Pack**, choose a plugin id (for example `mod_sync`).
3. Download the starter ZIP.
4. Edit the generated files:
   - `<id>/manifest.json`
   - `<id>/entrypoint.sh`
   - `<id>/event.sample.json`
   - `<id>/README.md`
5. Return to `/admin/plugins` and save the manifest.

You can also download directly via:

- `/admin/plugins/starter?id=mod_sync`

## Manifest contract

The starter manifest uses the same contract enforced by `/admin/plugins`:

- `id`: lowercase, stable, safe characters.
- `entrypoint`: explicit runtime entrypoint path.
- `capabilities`: must be in the approved capability list.
- `sandbox_profile`: explicit execution policy (`strict`, `network-limited`, `filesystem-readonly`, `none`).
- `retention_days`: bounded retention policy.

## Operational guidance

- Start with least privilege capabilities.
- Keep `sandbox_profile` strict by default.
- Treat starter files as templates; review before production use.
- Validate every manifest update through `/admin/plugins` so changes stay auditable.
