package usecase_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/interfaces"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model/config"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/types"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/memory"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/errutil"
)

// fakeEmbedClient produces a deterministic 3-axis embedding: index 0 = "github",
// 1 = "secret", 2 = "npm". This makes cosine ranking testable without a real
// embedding model.
type fakeEmbedClient struct{ calls int }

func (f *fakeEmbedClient) GenerateEmbedding(ctx context.Context, dimension int, input []string) ([][]float64, error) {
	f.calls++
	out := make([][]float64, len(input))
	for i, s := range input {
		v := make([]float64, dimension)
		ls := strings.ToLower(s)
		if strings.Contains(ls, "github") {
			v[0] = 1
		}
		if strings.Contains(ls, "secret") {
			v[1] = 1
		}
		if strings.Contains(ls, "npm") {
			v[2] = 1
		}
		out[i] = v
	}
	return out, nil
}

func newWS() string { return fmt.Sprintf("ws-%d", time.Now().UnixNano()) }

// createTestTag is a helper to create a tag via TagUseCase and return its ID.
func createTestTag(t *testing.T, ctx context.Context, tagUC *usecase.TagUseCase, ws, name string) model.TagID {
	t.Helper()
	tag, err := tagUC.CreateTag(ctx, ws, name)
	gt.NoError(t, err).Required()
	return tag.ID
}

func TestKnowledgeUseCase_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, nil, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "GitHub policy",
		Claim:  "use github actions with sha pin",
		TagIDs: []model.TagID{opsID, githubID, opsID, ""}, // opsID duped, empty dropped
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.ID).NotEqual(model.KnowledgeID(""))

	// TagIDs normalized (dedupe/drop-empty), order preserved.
	gt.Array(t, created.TagIDs).Length(2).Required()
	gt.Value(t, created.TagIDs[0]).Equal(opsID)
	gt.Value(t, created.TagIDs[1]).Equal(githubID)

	// Embedding generated from title+claim ("github" present -> axis 0).
	gt.Array(t, created.Embedding).Length(model.EmbeddingDimension).Required()
	gt.Value(t, created.Embedding[0]).Equal(float64(1))
	gt.Number(t, embed.calls).Equal(1)

	// Persisted and readable back.
	got, err := uc.GetKnowledge(ctx, ws, created.ID)
	gt.NoError(t, err).Required()
	gt.String(t, got.Title).Equal("GitHub policy")
	gt.String(t, got.Claim).Equal("use github actions with sha pin")
	gt.Array(t, got.TagIDs).Length(2).Required()
	gt.Value(t, got.TagIDs[0]).Equal(opsID)
	gt.Value(t, got.TagIDs[1]).Equal(githubID)
}

func TestKnowledgeUseCase_CreateInputValidation(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	t.Run("empty title", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "  ", TagIDs: []model.TagID{opsID}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
	t.Run("no tags", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", TagIDs: []model.TagID{"", ""}})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
	})
}

func TestKnowledgeUseCase_CreateUnknownTagFails(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	ws := newWS()

	// Pass a TagID that was never created in the workspace.
	nonExistent := model.NewTagID()
	_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "some title",
		Claim:  "some body",
		TagIDs: []model.TagID{nonExistent},
	})
	gt.Error(t, err).Is(usecase.ErrUnknownTag)
}

// TestKnowledgeUseCase_InputRejectionsAreBenign pins that a rejection of what the
// caller sent is demoted out of the error tracker. The agent composes these
// arguments itself, and the strategies report every failed tool call, so without
// the tag a model that invents a tag id files one issue per attempt. The
// assertion is on the error the usecase actually returns, since that is the value
// errutil.Handle receives after the tool and strategy layers wrap it.
func TestKnowledgeUseCase_InputRejectionsAreBenign(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	t.Run("an unknown tag id is benign", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
			Title:  "some title",
			Claim:  "some body",
			TagIDs: []model.TagID{model.NewTagID()},
		})
		gt.Error(t, err).Is(usecase.ErrUnknownTag)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("invalid input is benign", func(t *testing.T) {
		_, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
			Title:  "  ",
			TagIDs: []model.TagID{opsID},
		})
		gt.Error(t, err).Is(usecase.ErrKnowledgeInput)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("an unknown tag id on update is benign", func(t *testing.T) {
		created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
			Title:  "some knowledge",
			Claim:  "body",
			TagIDs: []model.TagID{opsID},
		})
		gt.NoError(t, err).Required()

		newTagIDs := []model.TagID{model.NewTagID()}
		_, err = uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
			ID:     created.ID,
			TagIDs: &newTagIDs,
		})
		gt.Error(t, err).Is(usecase.ErrUnknownTag)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()
	})

	t.Run("a repository failure inside tag verification stays reportable", func(t *testing.T) {
		// verifyTagsExist raises both the unknown-tag rejection and a backend
		// failure. Only the caller-input half is demoted; a broken Tag() backend
		// must still page, or the demotion has hidden a real outage.
		backendErr := goerr.New("tag backend unavailable")
		failUC := usecase.NewKnowledgeUseCase(
			&tagListFailureRepo{Repository: memory.New(), err: backendErr}, nil, nil)
		_, err := failUC.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
			Title:  "some title",
			Claim:  "some body",
			TagIDs: []model.TagID{opsID},
		})
		gt.Error(t, err).Is(backendErr)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).False()
	})
}

// tagListFailureRepo makes Tag().List fail while leaving every other repository
// intact, so the non-input failure path of tag verification can be exercised.
type tagListFailureRepo struct {
	interfaces.Repository
	err error
}

func (r *tagListFailureRepo) Tag() interfaces.TagRepository {
	return &failingTagRepo{TagRepository: r.Repository.Tag(), err: r.err}
}

type failingTagRepo struct {
	interfaces.TagRepository
	err error
}

func (r *failingTagRepo) List(_ context.Context, _ string) ([]*model.Tag, error) {
	return nil, r.err
}

func TestKnowledgeUseCase_CreateFailOpenWithoutEmbedClient(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "no embed",
		Claim:  "body",
		TagIDs: []model.TagID{opsID},
	})
	gt.NoError(t, err).Required()
	gt.Array(t, created.Embedding).Length(0)
}

func TestKnowledgeUseCase_Update(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, nil, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	npmID := createTestTag(t, ctx, tagUC, ws, "npm")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")
	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "npm policy",
		Claim:  "min release age",
		TagIDs: []model.TagID{npmID},
	})
	gt.NoError(t, err).Required()
	gt.Value(t, created.Embedding[2]).Equal(float64(1)) // npm axis

	newTitle := "github policy"
	newTagIDs := []model.TagID{githubID, opsID}
	updated, err := uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
		ID:     created.ID,
		Title:  &newTitle,
		TagIDs: &newTagIDs,
	})
	gt.NoError(t, err).Required()
	gt.String(t, updated.Title).Equal("github policy")
	gt.Array(t, updated.TagIDs).Length(2).Required()
	gt.Value(t, updated.TagIDs[0]).Equal(githubID)
	gt.Value(t, updated.TagIDs[1]).Equal(opsID)
	// Title changed -> re-embedded; "github" now present (axis 0), npm gone.
	gt.Value(t, updated.Embedding[0]).Equal(float64(1))
	gt.Value(t, updated.Embedding[2]).Equal(float64(0))
	gt.Bool(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt)).True()
}

func TestKnowledgeUseCase_UpdateUnknownTagFails(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{
		Title:  "some knowledge",
		Claim:  "body",
		TagIDs: []model.TagID{opsID},
	})
	gt.NoError(t, err).Required()

	nonExistent := model.NewTagID()
	newTagIDs := []model.TagID{nonExistent}
	_, err = uc.UpdateKnowledge(ctx, ws, usecase.UpdateKnowledgeInput{
		ID:     created.ID,
		TagIDs: &newTagIDs,
	})
	gt.Error(t, err).Is(usecase.ErrUnknownTag)
}

func TestKnowledgeUseCase_SearchSemanticRanking(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	embed := &fakeEmbedClient{}
	uc := usecase.NewKnowledgeUseCase(repo, nil, embed)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")
	githubID := createTestTag(t, ctx, tagUC, ws, "github")
	secretID := createTestTag(t, ctx, tagUC, ws, "secret")
	npmID := createTestTag(t, ctx, tagUC, ws, "npm")

	k1, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "github actions", TagIDs: []model.TagID{opsID, githubID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "Secret handling", Claim: "rotate secret tokens", TagIDs: []model.TagID{opsID, secretID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "npm policy", Claim: "min release age", TagIDs: []model.TagID{npmID}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github actions"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(3).Required()
	gt.Value(t, res[0].ID).Equal(k1.ID) // highest cosine

	// Tag pre-filter narrows the candidate set.
	resOps, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()
	gt.Array(t, resOps).Length(2).Required()
	gt.Value(t, resOps[0].ID).Equal(k1.ID)

	// Limit caps results.
	resLimited, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github", Limit: 1})
	gt.NoError(t, err).Required()
	gt.Array(t, resLimited).Length(1)
}

func TestKnowledgeUseCase_SearchSubstringFallback(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil) // no embed client
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	hit, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "GitHub policy", Claim: "body", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()
	_, err = uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "unrelated", Claim: "nothing here", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()

	res, err := uc.SearchKnowledge(ctx, ws, usecase.SearchKnowledgeInput{Query: "github"})
	gt.NoError(t, err).Required()
	gt.Array(t, res).Length(2).Required()
	gt.Value(t, res[0].ID).Equal(hit.ID) // substring match on title ranks first
}

func TestKnowledgeUseCase_Delete(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	uc := usecase.NewKnowledgeUseCase(repo, nil, nil)
	tagUC := usecase.NewTagUseCase(repo)
	ws := newWS()

	opsID := createTestTag(t, ctx, tagUC, ws, "ops")

	created, err := uc.CreateKnowledge(ctx, ws, usecase.CreateKnowledgeInput{Title: "x", TagIDs: []model.TagID{opsID}})
	gt.NoError(t, err).Required()

	gt.NoError(t, uc.DeleteKnowledge(ctx, ws, created.ID)).Required()

	_, err = uc.GetKnowledge(ctx, ws, created.ID)
	gt.Error(t, err)

	// List reflects deletion.
	items, err := uc.ListKnowledge(ctx, ws, interfaces.KnowledgeListOptions{})
	gt.NoError(t, err).Required()
	gt.Array(t, items).Length(0)
}

// knowledgeKindsRegistry registers two workspaces: one with a [knowledge]
// section (kinds "note" and "glossary", default "note") and one without, which
// has only the implicit kind.
func knowledgeKindsRegistry(configuredWS, implicitWS string) *model.WorkspaceRegistry {
	reg := model.NewWorkspaceRegistry()
	reg.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: configuredWS, Name: "Configured"},
		KnowledgeConfig: &config.KnowledgeConfig{
			DefaultKindID: "note",
			Kinds: []config.KnowledgeKind{
				{ID: "note", Name: "Note", FieldSchema: &config.FieldSchema{Fields: []config.FieldDefinition{}}},
				{ID: "glossary", Name: "Glossary", FieldSchema: &config.FieldSchema{Fields: []config.FieldDefinition{
					{ID: "aliases", Name: "Aliases", Type: types.FieldTypeText, Required: true},
					{ID: "scope", Name: "Scope", Type: types.FieldTypeSelect, Options: []config.FieldOption{
						{ID: "team", Name: "Team"},
						{ID: "org", Name: "Organization"},
					}},
					{ID: "owner", Name: "Owner", Type: types.FieldTypeUser},
				}}},
			},
		},
	})
	reg.Register(&model.WorkspaceEntry{
		Workspace: model.Workspace{ID: implicitWS, Name: "Implicit"},
	})
	return reg
}

func TestKnowledgeUseCase_Kinds(t *testing.T) {
	ctx := context.Background()
	configuredWS := newWS() + "-configured"
	implicitWS := newWS() + "-implicit"

	setup := func(t *testing.T) (*usecase.KnowledgeUseCase, interfaces.Repository, model.TagID, model.TagID) {
		t.Helper()
		repo := memory.New()
		uc := usecase.NewKnowledgeUseCase(repo, knowledgeKindsRegistry(configuredWS, implicitWS), nil)
		tagUC := usecase.NewTagUseCase(repo)
		gt.NoError(t, repo.SlackUser().SaveMany(ctx, []*model.SlackUser{
			{ID: "U000ALICE", Name: "alice"},
		})).Required()
		return uc, repo,
			createTestTag(t, ctx, tagUC, configuredWS, "ops"),
			createTestTag(t, ctx, tagUC, implicitWS, "ops")
	}

	t.Run("a workspace without [knowledge] stores an empty kind and no fields", func(t *testing.T) {
		uc, repo, _, implicitTag := setup(t)

		created, err := uc.CreateKnowledge(ctx, implicitWS, usecase.CreateKnowledgeInput{
			Title:  "implicit",
			TagIDs: []model.TagID{implicitTag},
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, implicitWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.KindID).Equal("")
		gt.Number(t, len(got.FieldValues)).Equal(0)
	})

	t.Run("the implicit kind may be named explicitly and is still stored empty", func(t *testing.T) {
		uc, repo, _, implicitTag := setup(t)

		created, err := uc.CreateKnowledge(ctx, implicitWS, usecase.CreateKnowledgeInput{
			Title:  "implicit named",
			TagIDs: []model.TagID{implicitTag},
			KindID: config.ImplicitKnowledgeKindID,
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, implicitWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.KindID).Equal("")
	})

	t.Run("the implicit kind has no fields, so any field value is rejected", func(t *testing.T) {
		uc, repo, _, implicitTag := setup(t)

		_, err := uc.CreateKnowledge(ctx, implicitWS, usecase.CreateKnowledgeInput{
			Title:  "implicit with fields",
			TagIDs: []model.TagID{implicitTag},
			FieldValues: map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "x"},
			},
		})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()

		all, err := repo.Knowledge().List(ctx, implicitWS, interfaces.KnowledgeListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(0)
	})

	t.Run("a create naming no kind stores the explicit default kind", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		created, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "default kind",
			TagIDs: []model.TagID{configuredTag},
		})
		gt.NoError(t, err).Required()
		gt.String(t, created.KindID).Equal("note")

		got, err := repo.Knowledge().Get(ctx, configuredWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.KindID).Equal("note")
	})

	t.Run("a create with a kind stores validated, typed field values", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		created, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "Sample term",
			Claim:  "Sample definition",
			TagIDs: []model.TagID{configuredTag},
			KindID: "glossary",
			FieldValues: map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "sample alias"},
				"scope":   {FieldID: "scope", Value: "team"},
				"owner":   {FieldID: "owner", Value: "U000ALICE"},
			},
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, configuredWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.KindID).Equal("glossary")
		gt.Number(t, len(got.FieldValues)).Equal(3)
		gt.Value(t, got.FieldValues["aliases"].Type).Equal(types.FieldTypeText)
		gt.Value(t, got.FieldValues["aliases"].Value).Equal("sample alias")
		gt.Value(t, got.FieldValues["scope"].Type).Equal(types.FieldTypeSelect)
		gt.Value(t, got.FieldValues["scope"].Value).Equal("team")
		gt.Value(t, got.FieldValues["owner"].Type).Equal(types.FieldTypeUser)
		gt.Value(t, got.FieldValues["owner"].Value).Equal("U000ALICE")
	})

	t.Run("an unknown kind is rejected as benign and nothing is written", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		_, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "bad kind",
			TagIDs: []model.TagID{configuredTag},
			KindID: "missing",
		})
		gt.Error(t, err).Is(usecase.ErrUnknownKnowledgeKind)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()

		// The implicit kind's id means nothing once kinds are configured.
		_, err = uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "implicit id",
			TagIDs: []model.TagID{configuredTag},
			KindID: config.ImplicitKnowledgeKindID,
		})
		gt.Error(t, err).Is(usecase.ErrUnknownKnowledgeKind)

		all, err := repo.Knowledge().List(ctx, configuredWS, interfaces.KnowledgeListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(0)
	})

	t.Run("create enforces required fields, options, unknown ids and users", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		cases := []struct {
			name   string
			fields map[string]model.FieldValue
			want   error
		}{
			{"missing required", map[string]model.FieldValue{}, model.ErrCaseFieldValidation},
			{"invalid option", map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "a"},
				"scope":   {FieldID: "scope", Value: "nope"},
			}, model.ErrCaseFieldValidation},
			{"unknown field id", map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "a"},
				"extra":   {FieldID: "extra", Value: "x"},
			}, model.ErrCaseFieldValidation},
			{"unknown user", map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "a"},
				"owner":   {FieldID: "owner", Value: "U000GHOST"},
			}, usecase.ErrUnknownUser},
		}
		for _, tc := range cases {
			_, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
				Title:       tc.name,
				TagIDs:      []model.TagID{configuredTag},
				KindID:      "glossary",
				FieldValues: tc.fields,
			})
			gt.Error(t, err).Is(tc.want)
		}

		all, err := repo.Knowledge().List(ctx, configuredWS, interfaces.KnowledgeListOptions{})
		gt.NoError(t, err).Required()
		gt.Array(t, all).Length(0)
	})

	t.Run("update merges a field patch and keeps the kind", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		created, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "term",
			TagIDs: []model.TagID{configuredTag},
			KindID: "glossary",
			FieldValues: map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "first"},
				"scope":   {FieldID: "scope", Value: "team"},
			},
		})
		gt.NoError(t, err).Required()

		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID: created.ID,
			FieldValues: map[string]model.FieldValue{
				"scope": {FieldID: "scope", Value: "org"},
			},
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, configuredWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.KindID).Equal("glossary")
		gt.Number(t, len(got.FieldValues)).Equal(2)
		gt.Value(t, got.FieldValues["aliases"].Value).Equal("first")
		gt.Value(t, got.FieldValues["scope"].Value).Equal("org")
		gt.Value(t, got.FieldValues["scope"].Type).Equal(types.FieldTypeSelect)
	})

	t.Run("update rejects an invalid patch and leaves the entry untouched", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)

		created, err := uc.CreateKnowledge(ctx, configuredWS, usecase.CreateKnowledgeInput{
			Title:  "term",
			TagIDs: []model.TagID{configuredTag},
			KindID: "glossary",
			FieldValues: map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "first"},
			},
		})
		gt.NoError(t, err).Required()

		newTitle := "renamed"
		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID:    created.ID,
			Title: &newTitle,
			FieldValues: map[string]model.FieldValue{
				"unknown": {FieldID: "unknown", Value: "x"},
			},
		})
		gt.Error(t, err).Is(model.ErrCaseFieldValidation)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()

		got, err := repo.Knowledge().Get(ctx, configuredWS, created.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Title).Equal("term")
		gt.Number(t, len(got.FieldValues)).Equal(1)
	})

	t.Run("an entry written before kinds existed stays editable", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)
		now := time.Now().UTC()

		// A legacy entry: empty kind, no fields. It resolves to default_kind.
		legacy := &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: configuredWS,
			Title:       "legacy",
			TagIDs:      []model.TagID{configuredTag},
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := repo.Knowledge().Create(ctx, configuredWS, legacy)
		gt.NoError(t, err).Required()

		newTitle := "legacy edited"
		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID:    legacy.ID,
			Title: &newTitle,
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, configuredWS, legacy.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Title).Equal("legacy edited")
		// The stored kind is left as written; it keeps resolving to default_kind.
		gt.String(t, got.KindID).Equal("")
	})

	t.Run("a title edit does not re-demand a field that became required later", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)
		now := time.Now().UTC()

		stale := &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: configuredWS,
			Title:       "stale",
			TagIDs:      []model.TagID{configuredTag},
			KindID:      "glossary",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := repo.Knowledge().Create(ctx, configuredWS, stale)
		gt.NoError(t, err).Required()

		newTitle := "stale edited"
		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID:    stale.ID,
			Title: &newTitle,
		})
		gt.NoError(t, err).Required()

		// A patch is validated on its own, so the missing required field is
		// still not demanded.
		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID: stale.ID,
			FieldValues: map[string]model.FieldValue{
				"scope": {FieldID: "scope", Value: "org"},
			},
		})
		gt.NoError(t, err).Required()

		got, err := repo.Knowledge().Get(ctx, configuredWS, stale.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Title).Equal("stale edited")
		gt.Value(t, got.FieldValues["scope"].Value).Equal("org")
	})

	t.Run("an entry whose kind was removed can be edited, but not its fields", func(t *testing.T) {
		uc, repo, configuredTag, _ := setup(t)
		now := time.Now().UTC()

		orphan := &model.Knowledge{
			ID:          model.NewKnowledgeID(),
			WorkspaceID: configuredWS,
			Title:       "orphan",
			TagIDs:      []model.TagID{configuredTag},
			KindID:      "removed_kind",
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		_, err := repo.Knowledge().Create(ctx, configuredWS, orphan)
		gt.NoError(t, err).Required()

		newTitle := "orphan edited"
		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID:    orphan.ID,
			Title: &newTitle,
		})
		gt.NoError(t, err).Required()

		_, err = uc.UpdateKnowledge(ctx, configuredWS, usecase.UpdateKnowledgeInput{
			ID: orphan.ID,
			FieldValues: map[string]model.FieldValue{
				"aliases": {FieldID: "aliases", Value: "x"},
			},
		})
		gt.Error(t, err).Is(usecase.ErrUnknownKnowledgeKind)
		gt.Bool(t, goerr.HasTag(err, errutil.TagBenign)).True()

		got, err := repo.Knowledge().Get(ctx, configuredWS, orphan.ID)
		gt.NoError(t, err).Required()
		gt.String(t, got.Title).Equal("orphan edited")
		gt.String(t, got.KindID).Equal("removed_kind")
		gt.Number(t, len(got.FieldValues)).Equal(0)
	})

	t.Run("a workspace the registry does not know is an error", func(t *testing.T) {
		uc, _, _, _ := setup(t)
		_, err := uc.CreateKnowledge(ctx, newWS()+"-unknown", usecase.CreateKnowledgeInput{
			Title:  "x",
			TagIDs: []model.TagID{model.NewTagID()},
		})
		gt.Error(t, err).Is(model.ErrWorkspaceNotFound)
	})
}
