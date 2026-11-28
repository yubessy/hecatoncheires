package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/m-mizutani/gt"
	"github.com/m-mizutani/hecatoncheires/pkg/agent/tool/casewriter"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model"
	"github.com/m-mizutani/hecatoncheires/pkg/domain/model/auth"
	"github.com/m-mizutani/hecatoncheires/pkg/repository/memory"
	"github.com/m-mizutani/hecatoncheires/pkg/usecase"
)

func TestCaseToolAdapter_UpdateCaseCarriesTheDueDate(t *testing.T) {
	gt.Value(t, usecase.NewCaseToolAdapter(nil)).Nil()

	repo := memory.New()
	registry := model.NewWorkspaceRegistry()
	registry.Register(&model.WorkspaceEntry{
		Workspace:   model.Workspace{ID: testWorkspaceID},
		CaseDueDate: true,
	})
	uc := usecase.NewCaseUseCase(repo, registry, nil, nil, "")
	ctx := auth.ContextWithToken(context.Background(), &auth.Token{Sub: "U-ADAPTER"})
	created, err := uc.CreateCase(ctx, testWorkspaceID, "Sample case 1", "", nil, nil, false, false, "", "")
	gt.NoError(t, err).Required()

	adapter := usecase.NewCaseToolAdapter(uc)
	due := time.Date(2030, 6, 30, 0, 0, 0, 0, time.UTC)
	title := "Sample case 1 renamed"
	_, err = adapter.UpdateCase(ctx, testWorkspaceID, created.ID, casewriter.CaseUpdate{Title: &title, DueDate: &due})
	gt.NoError(t, err).Required()
	stored, err := repo.Case().Get(ctx, testWorkspaceID, created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.Title).Equal(title)
	gt.Value(t, stored.DueDate).NotNil().Required()
	gt.Bool(t, stored.DueDate.Equal(due)).True()

	_, err = adapter.UpdateCase(ctx, testWorkspaceID, created.ID, casewriter.CaseUpdate{ClearDueDate: true})
	gt.NoError(t, err).Required()
	stored, err = repo.Case().Get(ctx, testWorkspaceID, created.ID)
	gt.NoError(t, err).Required()
	gt.Value(t, stored.DueDate).Nil()
}
