package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jhonataT/ai-decrivo-golang/internal/debt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type StartDebtInput struct {
	RepoPath string         `json:"repoPath" jsonschema:"caminho absoluto do repositório"`
	Base     string         `json:"base" jsonschema:"branch base, normalmente main"`
	Branch   string         `json:"branch,omitempty" jsonschema:"opcional: mapeia só as dívidas que esta branch introduz (arquivos do diff contra a base). Vazio: o projeto inteiro no estado da base"`
	Labels   []debt.Label   `json:"labels,omitempty" jsonschema:"labels existentes no repositório (gh label list); quem revisa escolhe entre elas"`
	Projects []debt.Project `json:"projects,omitempty" jsonschema:"projetos do GitHub do dono do repositório (gh project list); quem revisa pode escolher um"`
}

type ScanInput struct {
	ScanID string `json:"scanId" jsonschema:"id devolvido por start_debt_scan"`
}

type AddDebtInput struct {
	ScanID      string   `json:"scanId"`
	Title       string   `json:"title" jsonschema:"frase curta que sirva de título de issue"`
	Description string   `json:"description" jsonschema:"o que é a dívida e o que fazer para pagá-la"`
	Reason      string   `json:"reason" jsonschema:"por que é dívida: o custo ou risco concreto de deixar como está"`
	File        string   `json:"file" jsonschema:"caminho relativo à raiz do repositório"`
	StartLine   int      `json:"startLine,omitempty" jsonschema:"primeira linha do trecho; 0 para o arquivo como um todo"`
	EndLine     int      `json:"endLine,omitempty" jsonschema:"última linha do trecho; 0 para usar startLine"`
	EffortHours float64  `json:"effortHours" jsonschema:"esforço estimado para pagar a dívida, em horas"`
	Labels      []string `json:"labels,omitempty" jsonschema:"labels sugeridas, só entre as informadas em start_debt_scan"`
}

type FinishDebtInput struct {
	ScanID  string `json:"scanId"`
	Summary string `json:"summary" jsonschema:"2 a 4 frases sobre o panorama das dívidas encontradas, para quem vai priorizar"`
}

type PublishableDebtsInput struct {
	ScanID  string `json:"scanId"`
	RepoURL string `json:"repoUrl" jsonschema:"URL do repositório no GitHub (gh repo view --json url), para o link fixo do trecho"`
}

type MarkDebtInput struct {
	ScanID   string `json:"scanId"`
	DebtID   string `json:"debtId"`
	IssueURL string `json:"issueUrl" jsonschema:"URL da issue criada"`
}

// publishable é a resposta de get_publishable_debts.
type publishable struct {
	RepoPath      string       `json:"repoPath"`
	Commit        string       `json:"commit"`
	Project       string       `json:"project,omitempty"`
	ProjectNumber int          `json:"projectNumber,omitempty"`
	Issues        []debt.Issue `json:"issues"`
}

func addDebtTools(s *mcp.Server, debts *debt.Service, onReady func(scanID string)) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "start_debt_scan",
		Description: "Start a technical debt scan of a repository and return the scanId. Without branch, the whole " +
			"project at the base branch is in scope; with branch, only the files changed against base. Pass the " +
			"repository labels and GitHub projects so the human reviewer can pick among them.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in StartDebtInput) (*mcp.CallToolResult, any, error) {
		sc, err := debts.Start(ctx, debt.StartInput{
			Repo: in.RepoPath, Base: in.Base, Branch: in.Branch, Labels: in.Labels, Projects: in.Projects,
		})
		if err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		fmt.Fprintf(&b, "scanId=%s\ncommit=%s\nescopo: %s\n", sc.ID, sc.Commit, sc.Scope())
		if len(sc.Files) > 0 {
			b.WriteString("arquivos no escopo:\n")
			for _, f := range sc.Files {
				fmt.Fprintf(&b, "- %s\n", f)
			}
		}
		return text(b.String()), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "add_debt",
		Description: "Register one technical debt. Lines refer to the file at the scanned commit. " +
			"The human reviewer will accept, adjust or reject it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AddDebtInput) (*mcp.CallToolResult, any, error) {
		id, err := debts.AddDebt(ctx, in.ScanID, debt.Debt{
			Text: debt.Text{
				Title: in.Title, Description: in.Description, Reason: in.Reason,
				EffortHours: in.EffortHours, Labels: in.Labels,
			},
			File: in.File, StartLine: in.StartLine, EndLine: in.EndLine,
		})
		if err != nil {
			return nil, nil, err
		}
		return text("debtId=" + id), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "finish_debt_scan",
		Description: "Signal that the scan is done, with an overview. Brings the Decrivo window to the front for the human verdicts.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in FinishDebtInput) (*mcp.CallToolResult, any, error) {
		if err := debts.MarkReady(in.ScanID, in.Summary); err != nil {
			return nil, nil, err
		}
		onReady(in.ScanID)
		sc, err := debts.Get(in.ScanID)
		if err != nil {
			return nil, nil, err
		}
		return text(fmt.Sprintf("mapeamento pronto: %d dívidas (%s) aguardando veredito humano no Decrivo",
			sc.Pending(), debt.Hours(sc.TotalHours()))), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "get_publishable_debts",
		Description: "After the human finalized the scan and checked \"criar issues\", return the issues to create: " +
			"only accepted debts without an issue yet, with title, body and labels ready. Fails if the scan is not " +
			"released. Create each one exactly as given (title, body, labels) and call mark_debt_published right after " +
			"each creation; when projectNumber is set, also add the issue to that project. Never edit titles, bodies or labels.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in PublishableDebtsInput) (*mcp.CallToolResult, any, error) {
		sc, err := debts.Get(in.ScanID)
		if err != nil {
			return nil, nil, err
		}
		issues, err := sc.Issues(in.RepoURL)
		if err != nil {
			return nil, nil, err
		}
		out := publishable{RepoPath: sc.Repo, Commit: sc.Commit, Project: sc.Project, Issues: issues}
		for _, p := range sc.Projects {
			if p.Title == sc.Project {
				out.ProjectNumber = p.Number
			}
		}
		js, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return text(string(js)), nil, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "mark_debt_published",
		Description: "Record the issue created for one debt, so it is never created twice.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in MarkDebtInput) (*mcp.CallToolResult, any, error) {
		if err := debts.MarkPublished(in.ScanID, in.DebtID, in.IssueURL); err != nil {
			return nil, nil, err
		}
		return text("ok"), nil, nil
	})
}
