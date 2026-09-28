package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/jhonataT/ai-decrivo-golang/internal/review"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StartInput struct {
	RepoPath string `json:"repoPath" jsonschema:"caminho absoluto do repositório"`
	Base     string `json:"base" jsonschema:"branch base, normalmente main"`
	Branch   string `json:"branch" jsonschema:"branch a revisar"`
}

type ReviewInput struct {
	ReviewID string `json:"reviewId" jsonschema:"id devolvido por start_review"`
}

type DiffInput struct {
	ReviewID string `json:"reviewId" jsonschema:"id devolvido por start_review"`
	File     string `json:"file" jsonschema:"caminho do arquivo, como listado por list_changed_files"`
}

type FileContextInput struct {
	ReviewID string `json:"reviewId"`
	File     string `json:"file"`
	Context  string `json:"context" jsonschema:"uma ou duas frases sobre o que o arquivo faz no projeto (a responsabilidade dele), não sobre o que mudou"`
}

type FinishInput struct {
	ReviewID       string `json:"reviewId"`
	Summary        string `json:"summary" jsonschema:"resumo direto do que a alteração entrega, em 2 a 4 frases, para quem vai aprovar o PR"`
	Recommendation string `json:"recommendation" jsonschema:"sugestão inicial: approve, request_changes ou comment"`
	Reason         string `json:"reason" jsonschema:"uma frase justificando a sugestão"`
}

type MarkPublishedInput struct {
	ReviewID string `json:"reviewId"`
	URL      string `json:"url" jsonschema:"html_url da review criada no GitHub"`
}

type AddFindingInput struct {
	ReviewID string `json:"reviewId"`
	File     string `json:"file"`
	Line     int    `json:"line" jsonschema:"linha no arquivo novo; 0 para comentário de arquivo"`
	Kind     string `json:"kind" jsonschema:"praise ou improvement"`
	Severity string `json:"severity" jsonschema:"info, minor ou major"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

// New expõe as ferramentas de revisão de PR e de mapeamento de dívidas.
// onReviewReady e onDebtReady trazem a janela para frente quando o agente termina.
func New(svc *review.Service, debts *debt.Service, onReviewReady, onDebtReady func(id string)) http.Handler {
	s := mcp.NewServer(&mcp.Implementation{Name: "decrivo"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "start_review",
		Description: "Start a branch review and return the reviewId and the changed files",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in StartInput) (*mcp.CallToolResult, any, error) {
		rev, err := svc.Start(ctx, in.RepoPath, in.Base, in.Branch)

		if err != nil {
			return nil, nil, err
		}

		return text(fmt.Sprintf("reviewId=%s\nfiles:\n%s", rev.ID, fileList(rev))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_changed_files",
		Description: "List the files changed in a review",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ReviewInput) (*mcp.CallToolResult, any, error) {
		rev, err := svc.Get(in.ReviewID)
		if err != nil {
			return nil, nil, err
		}
		return text(fileList(&rev)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_diff",
		Description: "Get the diff of one file. Each commentable line is prefixed with its line number " +
			"in the new file; use that number as `line` in add_finding. Removed lines have no number.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in DiffInput) (*mcp.CallToolResult, any, error) {
		rev, err := svc.Get(in.ReviewID)
		if err != nil {
			return nil, nil, err
		}
		file, ok := rev.File(in.File)
		if !ok {
			return nil, nil, fmt.Errorf("%w: %s", review.ErrFileNotInDiff, in.File)
		}
		return text(numberedDiff(file)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "add_finding",
		Description: "Register a finding anchored to a line of the new file (or line 0 for the whole file). " +
			"The human reviewer will accept or reject it.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in AddFindingInput) (*mcp.CallToolResult, any, error) {
		id, err := svc.AddFinding(in.ReviewID, review.Finding{
			File: in.File, Line: in.Line, Kind: in.Kind, Severity: in.Severity, Title: in.Title, Body: in.Body,
		})
		if err != nil {
			return nil, nil, err
		}
		return text("findingId=" + id), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "set_file_context",
		Description: "Describe, in one or two sentences, what a changed file is responsible for in the codebase " +
			"(its role, not the change). Shown to the reviewer above the file's diff.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FileContextInput) (*mcp.CallToolResult, any, error) {
		if err := svc.SetFileContext(in.ReviewID, in.File, in.Context); err != nil {
			return nil, nil, err
		}
		return text("ok"), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "finish_review",
		Description: "Signal that the analysis is done, with a summary of what the change delivers and an initial " +
			"recommendation (approve, request_changes or comment). Brings the Decrivo window to the front for the human verdicts.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FinishInput) (*mcp.CallToolResult, any, error) {
		fin := review.Finish{
			Summary:        in.Summary,
			Recommendation: review.Recommendation(in.Recommendation),
			Reason:         in.Reason,
		}
		if err := svc.MarkReady(in.ReviewID, fin); err != nil {
			return nil, nil, err
		}
		onReviewReady(in.ReviewID)

		rev, err := svc.Get(in.ReviewID)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("revisão pronta: %d achados aguardando veredito humano no Decrivo", rev.Pending())), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_publishable_review",
		Description: "After the human finalized the review in Decrivo and checked \"publicar no PR\", return the GitHub " +
			"pull request review payload with only the accepted findings. Fails if the review is not released for publishing. " +
			"To publish, run inside repoPath: save the JSON to a file; `gh pr view <branch> --json number,headRefOid,url`; " +
			"if headRefOid differs from commit_id, stop and tell the user (the branch changed after the review); otherwise " +
			"`gh api repos/{owner}/{repo}/pulls/<number>/reviews --method POST --input <file>` and call mark_published with the " +
			"html_url from the response. Post the payload as is: never edit comments or change the event.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in ReviewInput) (*mcp.CallToolResult, any, error) {
		rev, err := svc.Get(in.ReviewID)
		if err != nil {
			return nil, nil, err
		}
		payload, err := rev.GitHubReview()
		if err != nil {
			return nil, nil, err
		}
		js, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("repoPath=%s\nbranch=%s\npayload:\n%s", rev.Repo, rev.Branch, js)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mark_published",
		Description: "Record that the review was posted to the pull request, so it is not posted twice.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in MarkPublishedInput) (*mcp.CallToolResult, any, error) {
		if err := svc.MarkPublished(in.ReviewID, in.URL); err != nil {
			return nil, nil, err
		}
		return text("ok"), nil, nil
	})

	addDebtTools(s, debts, onDebtReady)

	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, nil)
}

func fileList(rev *review.Review) string {
	var b strings.Builder
	for _, f := range rev.Files {
		fmt.Fprintf(&b, "- %s\n", f.Path)
	}
	return b.String()
}

// numberedDiff formata o diff com o número da linha no arquivo novo à esquerda.
func numberedDiff(file review.FileChange) string {
	var b strings.Builder
	for _, l := range file.Lines {
		switch l.Kind {
		case "hunk":
			fmt.Fprintf(&b, "%s\n", l.Text)
		case "added":
			fmt.Fprintf(&b, "%5d +%s\n", l.NewLine, l.Text)
		case "removed":
			fmt.Fprintf(&b, "      -%s\n", l.Text)
		default:
			fmt.Fprintf(&b, "%5d  %s\n", l.NewLine, l.Text)
		}
	}
	return b.String()
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}
