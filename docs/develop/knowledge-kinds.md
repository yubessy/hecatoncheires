# Knowledge kinds and custom fields (design)

This document records the design of **knowledge kinds**: workspace-defined
categories of Knowledge, each carrying its own custom field schema. It is the
reference for the staged implementation listed at the end.

## Motivation

A Knowledge entry used to be a fixed shape — title, Markdown claim, tags. Some
workspaces want to keep more than one sort of shared knowledge (for example
operating rules next to a glossary of internal terms) and want each sort to
carry structured attributes, the way Case and Memo already do with
workspace-defined fields.

## Goals

- A workspace defines one or more **kinds** of Knowledge.
- Each kind defines its own custom fields, with the same `FieldDefinition`
  schema, types and validation as Case `[[fields]]` and `[[memo.fields]]`.
- **A workspace that does not opt in behaves exactly as before**, and no stored
  Knowledge has to be migrated — neither when the feature ships nor when a
  workspace later adds a `[knowledge]` section.

## Non-goals (for now)

- Changing a Knowledge entry's kind after creation.
- Per-kind tag vocabularies: tags stay workspace-wide.
- Including field values in the embedding used for semantic search.

## Configuration

```toml
[knowledge]
# The kind a Knowledge entry belongs to when it records none: every entry
# written before the workspace defined kinds, and a create that names no kind.
default_kind = "note"

[[knowledge.kinds]]
id = "note"
name = "Note"
description = "General operating knowledge."

[[knowledge.kinds]]
id = "glossary"
name = "Glossary"
description = "Definitions of internal terms."

[[knowledge.kinds.fields]]
id = "aliases"
name = "Aliases"
type = "text"
```

Validation at load time:

- `[knowledge]` present ⇒ at least one `[[knowledge.kinds]]`.
- A kind id matches `^[a-z][a-z0-9_]*$` (the field id format) and is unique in
  the workspace. A kind needs a `name`.
- `default_kind` is required and names one of the kinds.
- Each kind's fields follow every `[[fields]]` rule, field ids are unique within
  the kind, and `case_ref` / `multi_case_ref` are rejected — as in `[memo]`, the
  reference picker, tools and existence / privacy checks hang off the Case path.

### The implicit kind

A workspace without `[knowledge]` has a single **implicit kind** whose id is
`default` and which has no fields. That is precisely today's Knowledge, so such
a workspace sees no change.

## Data model

`model.Knowledge` gains two fields:

| Field | Meaning |
|---|---|
| `KindID string` | The kind the entry belongs to. **Empty means "the workspace's default kind".** |
| `FieldValues map[string]FieldValue` | Custom field values, keyed by field id — the same representation as Case and Memo. |

Stored documents are persisted as `*model.Knowledge` directly (repository write
contract), so both fields round-trip without any converter.

### How a kind id is stored

- In a workspace **without** `[knowledge]`, a new entry is stored with an
  **empty** `KindID`.
- In a workspace **with** `[knowledge]`, a new entry is stored with the
  **explicit** id of its kind (the requested one, or `default_kind` when none
  was requested).

The asymmetry is what lets a workspace add `[knowledge]` later without touching
data: every entry it already has carries an empty `KindID` and resolves to
`default_kind`. Had the implicit kind stored `"default"`, adding a `[knowledge]`
section without a kind of that id would orphan every existing entry. In the
other direction, storing the explicit id once kinds exist means a later change
of `default_kind` only re-homes the entries that never had a kind, not the ones
created under a specific kind.

Resolution is done in the usecase, never in the repository: the repository
stores and returns what it is given.

No new Firestore index is needed. Filtering by kind (stage 2) happens in memory,
like the existing tag filter.

## Write rules (usecase)

- **Create**: the requested kind (empty ⇒ default) must exist, otherwise
  `ErrUnknownKnowledgeKind` (benign — the caller asked for something the
  workspace does not define). Field values are validated with
  `FieldValidator.ValidateCaseFieldsAll` against that kind's schema: required
  fields enforced, unknown field ids rejected. User ids in `user` /
  `multi-user` fields must exist, as for Memo.
- **Update**: the kind cannot change. When the update carries field values they
  are validated with `ValidateCaseFieldsPartialStrict` against the stored
  entry's kind (unknown ids rejected, required fields not re-demanded) and then
  merged over the stored values. When it carries none, no field validation runs
  — so an entry written before a field became required can still have its title
  or claim edited, and an entry whose kind was removed from the configuration
  can still be edited as long as no field values are sent. Sending field values
  for an entry whose kind no longer exists fails with
  `ErrUnknownKnowledgeKind`.
- Field validation errors are tagged benign (they reject caller input, which
  for the agent is the model's own to repair).

## Consistency check (`validate --check-db`)

`ValidateDB` gains a Knowledge pass per workspace:

| Reported as | Detects |
|---|---|
| `knowledge_kind_invalid` | A stored, non-empty `KindID` that names no configured kind (the kind was removed or renamed, or `[knowledge]` was removed altogether). The same reasoning as `board_status_invalid`: the entry keeps pointing at a category that does not exist. An empty `KindID` always resolves — to `default_kind`, or to the implicit kind — so it is never reported. |
| `field_value` / `field_type_mismatch` | Knowledge field values checked against their kind's schema with `ValidateStored`, exactly as for Case and Memo. |

The sample target is `knowledge:<id>`; over HTTP it is
`{"kind": "knowledge", "knowledge_id": "<id>"}` with no `case_id`, because
Knowledge is not Case-scoped.

The existing exclusions apply unchanged: values under a removed field
definition and missing values of `required` fields are not reported.

## Staged implementation

1. **Configuration and domain** — `[knowledge]` parsing and validation, the
   domain `KnowledgeConfig`, `Knowledge.KindID` / `FieldValues`, usecase
   validation, repository round-trip tests, the `ValidateDB` pass, docs. No
   transport exposes kinds yet, so no visible behaviour changes.
2. **GraphQL and Web UI** — `knowledgeConfiguration(workspaceId)`, `kind` and
   `fields` on `Knowledge`, `kindId` / `fields` on the inputs, a `kindId`
   filter on `knowledges` / `searchKnowledge`, the list filter and the per-kind
   form.
3. **Agent tools** — `kind_id` and `fields` on `knowledge__create_knowledge` /
   `knowledge__update_knowledge`, both returned by get / search, the kinds and
   their fields described to the agent, every host that wires the knowledge
   toolset, and the eval tool catalog.
4. **BigQuery export** — `kind_id` and the field values on the `knowledge`
   table.
