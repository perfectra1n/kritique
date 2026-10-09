package worker

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"

	"github.com/perfectra1n/kritique/internal/adapter"
	"github.com/perfectra1n/kritique/internal/forge"
	"github.com/perfectra1n/kritique/internal/store"
	"github.com/perfectra1n/kritique/internal/tasks"
)

// taskSearchK is how many chunks a search source finds when it sets no k.
const taskSearchK = 8

// contextFile is one file of .Context.files.
type contextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// contextFiles reads the files the task names by path at the commit its
// definition came from. A glob needs a checkout: an agentic run's runner
// gathers it, and a single-mode run notes that it leaves it out.
func (r *taskRunner) contextFiles(ctx context.Context, budget *tasks.Budget) ([]contextFile, error) {
	var out []contextFile
	for _, f := range r.task.Context.Files {
		if f.Path == "" {
			if r.task.RunMode() != tasks.ModeAgentic {
				budget.Notes = append(budget.Notes, fmt.Sprintf("context files %s left out: a glob is gathered in agentic mode only", f.Glob))
			}
			continue
		}
		b, err := r.client.FileAt(ctx, r.owner, r.name, r.args.ConfigSHA, f.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			budget.Notes = append(budget.Notes, fmt.Sprintf("context files %s left out: not found", f.Path))
			continue
		case errors.Is(err, forge.ErrFileTooLarge):
			budget.Notes = append(budget.Notes, fmt.Sprintf("context files %s left out: too large", f.Path))
			continue
		case err != nil:
			return nil, err
		}
		out = append(out, contextFile{Path: f.Path, Content: budget.Take("files "+f.Path, string(b))})
	}
	return out, nil
}

// indexHit is one chunk of the repository index a search source found.
type indexHit struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Symbol    string `json:"symbol,omitempty"`
	Text      string `json:"text"`
}

// searchIndex finds the chunks of the repository's active index nearest q's
// query, nearest first. ok is false, and the reason noted, when there is
// no index to search.
func (r *taskRunner) searchIndex(ctx context.Context, q tasks.NamedQuery) (hits []indexHit, ok bool, err error) {
	w := r.w
	embedder, emb := w.Embedders.Embedder(r.file)
	if embedder == nil {
		r.notes = append(r.notes, fmt.Sprintf("context %s left out: repository indexing is off", q.Name))
		return nil, false, nil
	}
	if q.Query == "" {
		r.notes = append(r.notes, fmt.Sprintf("context %s left out: the query rendered empty", q.Name))
		return nil, false, nil
	}
	var runID string
	err = w.Store.WithAccount(ctx, r.account.ID(), func(tx pgx.Tx) error {
		var err error
		runID, err = store.ActiveGenerationFor(ctx, tx, r.args.RepositoryID, emb.Model, emb.Dims)
		return err
	})
	if errors.Is(err, store.ErrNotFound) {
		r.notes = append(r.notes, fmt.Sprintf("context %s left out: the repository has no index yet", q.Name))
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var vectors [][]float32
	var tokens int64
	err = w.withLease(ctx, r.account, "embed:"+emb.Model, r.settings.Limits.Concurrency, r.jobID, func(ctx context.Context) error {
		var err error
		vectors, tokens, err = embedder.Embed(ctx, []string{q.Query})
		w.Metrics.ModelCall(r.account.Key(), emb.Model, store.RoleEmbedding, adapter.Outcome(err), tokens, 0, 0, 0)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	if len(vectors) != 1 {
		return nil, false, fmt.Errorf("worker: embed search query: got %d vectors", len(vectors))
	}
	k := q.K
	if k <= 0 {
		k = taskSearchK
	}
	var found []store.SimilarHit
	err = w.Store.WithAccount(ctx, r.account.ID(), func(tx pgx.Tx) error {
		// A run past its answer never gathers context again; should one,
		// its embedding is not charged twice.
		if r.run.AnsweredAt == nil {
			if err := store.InsertUsage(ctx, tx, store.Usage{
				AccountID: r.account.ID(), RepositoryID: r.args.RepositoryID, Role: store.RoleEmbedding, Model: emb.Model, Input: tokens,
			}); err != nil {
				return err
			}
		}
		var err error
		found, err = store.SimilarChunks(ctx, tx, runID, vectors, []string{}, k)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	for _, h := range found {
		c := h.Chunk
		hits = append(hits, indexHit{Path: c.Path, StartLine: c.StartLine, EndLine: c.EndLine, Symbol: c.Symbol, Text: c.Text})
	}
	return hits, true, nil
}
