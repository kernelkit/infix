# Generic YANG-Driven Configuration System

## Objective

Design and implement a **generic, schema-driven configuration UI** for Infix using Go + HTMX.

The system must:

- Automatically render configuration from YANG models
- Use RESTCONF as the backend
- Avoid hardcoded UI logic for data representation
- Fall back to generic rendering for all non-handcrafted pages

Handcrafted UX is reserved for complex workflows only.

## Core Principles

1. **YANG is the single source of truth**
2. **No hardcoded values** (enums, identities, defaults, etc.)
3. **Schema-driven rendering**
4. **Lazy, scalable tree navigation**
5. **Strict alignment with sysrepo + RESTCONF semantics**

---

## Architecture Overview

### Backend (Go)

Responsibilities:

- RESTCONF proxy
- YANG schema retrieval and caching
- Optional YANG → internal schema translation layer

Endpoints (internal):

- `/api/schema` → processed YANG schema
- `/api/data?path=...` → fetch subtree
- `/api/commit` → apply changes

Avoid embedding business logic — rely on YANG.

---

### Frontend (HTMX)

- Tree navigation (left pane)
- Dynamic form renderer (right pane)
- Incremental updates via HTMX
- No SPA framework required

---

## Schema Handling

### Source

YANG modules are fetched from the target device at startup using two proven
RESTCONF endpoints (the same ones used by the Infamy test framework):

- **Module list**: `GET /restconf/data/ietf-yang-library:modules-state`
  Returns the list of implemented modules with name, revision, and features.
- **Schema files**: `GET /yang/{module}@{revision}.yang`
  Returns the raw YANG text for each module.

Submodules are fetched recursively. Schemas are cached on disk (keyed by
`{module}@{revision}.yang`) to avoid repeated downloads across restarts.

### YANG Parsing

The Go library **`github.com/openconfig/goyang`** is used to parse the
downloaded YANG files. It is pure Go (no CGo), handles identities,
enumerations, defaults, leafrefs, and the full YANG type system.

At startup, all implemented modules are loaded into a single `goyang` entry
map. This is required for correct identityref resolution: derived identities
are often defined in augmenting modules outside the module that declares the
base, so the full module set must be present before resolving any identity.

### Internal Representation

`goyang` produces an in-memory entry tree (`*yang.Entry`) that is the schema
layer's canonical form. A lightweight, JSON-serialisable struct is derived
from it for use by the frontend renderer:

```json
{
  "path": "/interfaces/interface",
  "kind": "list",
  "keys": ["name"],
  "children": [...],
  "type": null,
  "config": true,
  "mandatory": false,
  "default": null,
  "constraints": {
    "range": "...",
    "pattern": "..."
  }
}
```

This derived struct is what `/api/schema` serves; the raw `goyang` tree stays
server-side only.

---

## UI Design

### 1. Navigation Tree (Left Pane)

- Root: `/`
- Nodes:
  - container
  - list
  - list instances (keyed)

Behavior:

- Expand/collapse
- Lazy load children via `/api/data`
- Show only config-relevant nodes by default

---

### 2. Content View (Right Pane)

Render dynamically based on YANG node type.

#### Leaf Mapping

| YANG Type   | UI Element         |
| ----------- | ------------------ |
| boolean     | checkbox           |
| string      | text input         |
| int*        | number input       |
| enumeration | dropdown           |
| identityref | dropdown (dynamic) |
| leaf-list   | editable list      |

---

### 3. List Handling

- Show list entries as:
  - table OR expandable list
- Allow:
  - create (prompt for keys)
  - delete
  - edit per-entry

---

## YANG Semantics (Mandatory)

### config

- `config false` → read-only
- `config true` → editable

---

### default

- Must display effective value
- If unset:
  - show default (visually distinct)
- Do NOT write defaults unless explicitly changed

---

### mandatory

- Enforce before commit
- Highlight missing values

---

### constraints

#### range / length / pattern

- Validate client-side where possible
- Always rely on backend validation as final authority

---

### identityref (CRITICAL)

Must:

- Resolve base identity
- Enumerate all derived identities from the full loaded module set
- Populate dropdown dynamically

Example:

```yang
identity timezone;
identity Europe/Stockholm {
  base timezone;
}
```

UI must NOT hardcode timezone values. Derived identities may be spread across
multiple YANG modules — resolution requires the full module set to be loaded
(see Schema Handling above).

---

### leafref

- Resolve referenced path
- Validate selection
- Prefer dropdown if target is enumerable

---

## Data Flow

### Read

- GET `/api/data?path=/interfaces`
- Return JSON subtree

### Write

- Stage changes client-side
- Commit via:

#### PATCH (preferred)

- Partial updates

#### PUT

- Full replacement where required

#### DELETE

- Remove nodes or list entries

---

## State Management

- Maintain a client-side "candidate config"
- Track:
  - modified nodes
  - created/deleted list entries

Optional:

- diff view before commit

---

## Performance Requirements

- No full tree loads
- Lazy load everything
- Separate:
  - schema cache (static-ish)
  - data (dynamic)

---

## Extensibility

### Override Mechanism

Allow mapping:

/interfaces/interface → custom page
/system → custom page

Fallback:

- Generic renderer for all other paths

---

## Error Handling

- Surface RESTCONF errors clearly
- Map validation errors to fields
- Do not silently ignore failures

---

## Non-Goals

- No duplication of YANG logic in Go
- No hardcoded enums, identities, or defaults
- No full SPA frameworks

---

## Implementation Phases

### Phase 1a: Schema Infrastructure

- Add `openconfig/goyang` dependency
- Implement schema fetcher: download module list + YANG files from device
- Implement disk cache (`{module}@{revision}.yang`)
- Load all implemented modules into goyang entry map
- Expose `/api/schema?path=...` endpoint returning the derived JSON struct

This phase is backend-only and independently testable before any UI work.

---

### Phase 1b: Tree Navigation UI

- Implement tree navigation (containers + lists) in left pane
- Lazy load children via `/api/schema`
- Show config-relevant nodes only by default

---

### Phase 2: Basic Editing

- Render basic leaf types:
  - string, int, boolean
- Implement PATCH updates

---

### Phase 3: Advanced Types

- identityref (dynamic resolution from full module set)
- enumeration
- leaf-list
- list entry creation/deletion

**Acceptance test for this phase**: the following known regressions in the
handcrafted pages must be fixed as a side-effect of correct schema use:

- Configure > Users: shell field shows "CLI Shell" for admin instead of the
  YANG default
- Configure > System: timezone field blank instead of showing YANG default
- Configure > System: text editor shows "--not set--" instead of YANG default

---

### Phase 4: Validation

- Enforce:
  - mandatory
  - constraints
- Improve error mapping

---

### Phase 5: Polish

- Default value visualization
- Diff/preview before commit
- Custom page overrides

---

## Key Risks (Do Not Ignore)

- Incorrect identityref handling → breaks real configs
- Ignoring defaults → misleading UI
- Fetching full tree → performance collapse
- Hardcoding enums → defeats entire design

---

## Reference Behavior

Target parity with:

- sysrepo CLI workflows
- klish sysrepo plugin tree navigation

---

## Final Requirement

**If a new YANG model is added to the system, it must appear automatically in the UI without code changes.**
