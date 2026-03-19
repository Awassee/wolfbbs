# Roadmap 146-150 Kickoff

This kickoff starts Theme 16 (`146-150`) with delivery order intact and concrete execution contracts.

## Current Kickoff Scope

1. Item 150 alpha: release dashboard route in web admin.
   - Route: `/admin/release`
   - Purpose: unify roadmap status, QA evidence, release notes, and packaging commands in one operator view.
2. Items 146-149 foundation contracts documented here for implementation sequencing.

## Delivery Update (Shipped)

- 146 shipped at `/admin/plugins` with explicit manifest validation, sandbox profile checks, and capability matrix.
- 147 shipped at `/admin/themes` with importable bundle contract, safe extracted bundle files, and apply workflow.
- 148 shipped at `/admin/webhooks` with endpoint/token/event controls plus retry/backoff delivery logs.
- 149 shipped at `/admin/analytics` with bounded daily/weekly/monthly KPI windows and recommendations.
- 150 follow-on shipped at `/admin/release` with persisted release checklist state and package artifact inspection.

## Item Contracts

### 146. Plugin manifest, capability model, sandbox contract
- Deliverables:
  - plugin manifest schema (`id`, `entrypoint`, `capabilities`, `retention`, `sandbox profile`)
  - validation pipeline for manifest ingestion
  - operator-visible capability matrix in admin UI
- Exit criteria:
  - invalid manifests fail with explicit errors
  - capability flags are auditable and visible

### 147. Theme marketplace / import path
- Deliverables:
  - importable theme bundle format (manifest + assets)
  - validation and safe extraction path
  - preview + apply workflow in admin
- Exit criteria:
  - bad bundles rejected safely
  - applied themes reflected in runtime settings and status

### 148. External webhook bridge for board events
- Deliverables:
  - outbound webhook event schema for board lifecycle events
  - retry/backoff and failure logging
  - operator controls for endpoint, token, and event selection
- Exit criteria:
  - delivery outcomes visible in ops/audit
  - test harness validates payload shape and retry semantics

### 149. Embedded product analytics summary for sysops
- Deliverables:
  - summary KPIs for retention/activity on boards/chat/doors/events
  - bounded historical windows (daily/weekly/monthly)
  - operator dashboard card set with plain-language recommendations
- Exit criteria:
  - metrics derive from existing persisted events only
  - no new PII exposure in rendered summaries

### 150. Release dashboard linking roadmap, QA, docs, and artifacts
- Kickoff status:
  - alpha shipped in `/admin/release`
- Next increments:
  - release checklist state persistence
  - tag/release metadata fetch and render
  - package artifact inspection integration
