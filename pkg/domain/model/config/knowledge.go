package config

// ImplicitKnowledgeKindID is the id of the single kind a workspace has when its
// configuration defines no [knowledge] section. That kind has no custom fields,
// which is exactly the Knowledge shape that predates kinds.
const ImplicitKnowledgeKindID = "default"

// KnowledgeKind is one workspace-defined category of Knowledge with its own
// custom field schema.
type KnowledgeKind struct {
	ID          string
	Name        string
	Description string
	// FieldSchema is the kind's custom field schema. It is never nil; a kind
	// without fields has an empty Fields slice.
	FieldSchema *FieldSchema
}

// KnowledgeConfig is a workspace's [knowledge] configuration.
//
// A nil *KnowledgeConfig is meaningful: it is a workspace that defines no
// [knowledge] section and therefore has the single implicit kind
// (ImplicitKnowledgeKindID, no fields). Every method is nil-safe and answers
// for that implicit kind, so callers never special-case the absent section.
type KnowledgeConfig struct {
	// DefaultKindID is the kind an entry belongs to when it records none:
	// every entry written before the workspace defined kinds, and a create that
	// names no kind. It always names one of Kinds (enforced at config load).
	DefaultKindID string
	// Kinds are the configured kinds, in configuration order. Never empty
	// (enforced at config load).
	Kinds []KnowledgeKind
}

// implicitKnowledgeKind is the kind of a workspace without [knowledge].
func implicitKnowledgeKind() KnowledgeKind {
	return KnowledgeKind{
		ID:          ImplicitKnowledgeKindID,
		FieldSchema: &FieldSchema{Fields: []FieldDefinition{}},
	}
}

// Configured reports whether the workspace defines a [knowledge] section. When
// it does not, the workspace has only the implicit kind.
func (c *KnowledgeConfig) Configured() bool {
	return c != nil
}

// EffectiveKinds returns the workspace's kinds: the configured ones, or the
// single implicit kind when [knowledge] is absent.
func (c *KnowledgeConfig) EffectiveKinds() []KnowledgeKind {
	if c == nil {
		return []KnowledgeKind{implicitKnowledgeKind()}
	}
	return c.Kinds
}

// ResolveKindID maps a kind id as stored (or as requested by a caller) to the
// effective kind id. An empty id means "the default kind".
func (c *KnowledgeConfig) ResolveKindID(kindID string) string {
	if kindID != "" {
		return kindID
	}
	if c == nil {
		return ImplicitKnowledgeKindID
	}
	return c.DefaultKindID
}

// Kind looks a kind up by id, resolving an empty id to the default kind. It
// reports false when the workspace defines no kind with that id.
func (c *KnowledgeConfig) Kind(kindID string) (*KnowledgeKind, bool) {
	resolved := c.ResolveKindID(kindID)
	for _, k := range c.EffectiveKinds() {
		if k.ID == resolved {
			return &k, true
		}
	}
	return nil, false
}

// StoredKindID returns the KindID a new entry of the given (resolved) kind is
// persisted with. A workspace without [knowledge] stores an empty id, so that
// adding a [knowledge] section later re-homes every existing entry onto
// default_kind instead of leaving it pointing at the implicit kind's id. A
// workspace with [knowledge] stores the explicit id, so a later change of
// default_kind moves only the entries that never had a kind.
func (c *KnowledgeConfig) StoredKindID(resolvedKindID string) string {
	if c == nil {
		return ""
	}
	return resolvedKindID
}
