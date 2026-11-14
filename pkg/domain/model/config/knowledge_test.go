package config_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model/config"
)

func configuredKnowledge() *config.KnowledgeConfig {
	return &config.KnowledgeConfig{
		DefaultKindID: "note",
		Kinds: []config.KnowledgeKind{
			{ID: "note", Name: "Note", FieldSchema: &config.FieldSchema{}},
			{ID: "glossary", Name: "Glossary", FieldSchema: &config.FieldSchema{
				Fields: []config.FieldDefinition{{ID: "aliases", Name: "Aliases", Type: "text"}},
			}},
		},
	}
}

func TestKnowledgeConfig_Implicit(t *testing.T) {
	var cfg *config.KnowledgeConfig

	gt.False(t, cfg.Configured())

	kinds := cfg.EffectiveKinds()
	gt.A(t, kinds).Length(1)
	gt.V(t, kinds[0].ID).Equal(config.ImplicitKnowledgeKindID)
	gt.V(t, kinds[0].FieldSchema).NotNil()
	gt.A(t, kinds[0].FieldSchema.Fields).Length(0)

	gt.V(t, cfg.ResolveKindID("")).Equal(config.ImplicitKnowledgeKindID)
	gt.V(t, cfg.ResolveKindID("other")).Equal("other")

	k, ok := cfg.Kind("")
	gt.True(t, ok)
	gt.V(t, k.ID).Equal(config.ImplicitKnowledgeKindID)

	k, ok = cfg.Kind(config.ImplicitKnowledgeKindID)
	gt.True(t, ok)
	gt.V(t, k.ID).Equal(config.ImplicitKnowledgeKindID)

	_, ok = cfg.Kind("note")
	gt.False(t, ok)

	// The implicit kind is stored as an empty id so that adding [knowledge]
	// later re-homes the entry onto default_kind.
	gt.V(t, cfg.StoredKindID(config.ImplicitKnowledgeKindID)).Equal("")
}

func TestKnowledgeConfig_Configured(t *testing.T) {
	cfg := configuredKnowledge()

	gt.True(t, cfg.Configured())
	gt.A(t, cfg.EffectiveKinds()).Length(2)

	gt.V(t, cfg.ResolveKindID("")).Equal("note")
	gt.V(t, cfg.ResolveKindID("glossary")).Equal("glossary")

	k, ok := cfg.Kind("")
	gt.True(t, ok)
	gt.V(t, k.ID).Equal("note")

	k, ok = cfg.Kind("glossary")
	gt.True(t, ok)
	gt.V(t, k.Name).Equal("Glossary")
	gt.A(t, k.FieldSchema.Fields).Length(1)

	_, ok = cfg.Kind("missing")
	gt.False(t, ok)

	// The implicit kind's id has no special meaning once kinds are configured.
	_, ok = cfg.Kind(config.ImplicitKnowledgeKindID)
	gt.False(t, ok)

	gt.V(t, cfg.StoredKindID("glossary")).Equal("glossary")
	gt.V(t, cfg.StoredKindID("note")).Equal("note")
}
