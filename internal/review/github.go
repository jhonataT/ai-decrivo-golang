package review

import (
	"fmt"
	"strings"
)

// GitHubReview é o corpo de POST /repos/{owner}/{repo}/pulls/{n}/reviews.
// O Decrivo só monta o payload; quem publica é o agente, com o gh.
type GitHubReview struct {
	CommitID string          `json:"commit_id"`
	Event    string          `json:"event"`
	Body     string          `json:"body"`
	Comments []GitHubComment `json:"comments"`
}

type GitHubComment struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Side string `json:"side"`
	Body string `json:"body"`
}

var githubEvents = map[Recommendation]string{
	RecommendApprove:        "APPROVE",
	RecommendRequestChanges: "REQUEST_CHANGES",
	RecommendComment:        "COMMENT",
}

// CanPublish diz se o agente já pode publicar a revisão no PR.
func (r *Review) CanPublish() error {
	switch {
	case r.Status != StatusFinalized:
		return fmt.Errorf("%w: a revisão ainda não foi finalizada (status: %s)", ErrNotPublishable, r.Status)
	case !r.Publish:
		return fmt.Errorf("%w: quem revisou não marcou \"publicar no PR\"", ErrNotPublishable)
	case !r.PublishedAt.IsZero():
		return fmt.Errorf("%w: já publicada em %s", ErrNotPublishable, r.PublishedURL)
	case r.HeadSHA == "":
		return fmt.Errorf("%w: revisão sem o commit revisado registrado", ErrNotPublishable)
	}
	return nil
}

// GitHubReview monta a review do PR só com os achados aceitos, já com o texto
// ajustado por quem revisou. Achados de arquivo (linha 0) não têm onde ancorar
// no endpoint de reviews e vão para o corpo.
func (r *Review) GitHubReview() (GitHubReview, error) {
	if err := r.CanPublish(); err != nil {
		return GitHubReview{}, err
	}

	var body strings.Builder
	body.WriteString(r.ChangeSummary)
	comments := []GitHubComment{}
	fileLevel := 0
	for _, f := range r.Findings {
		if f.Verdict != VerdictAccepted {
			continue
		}
		text := fmt.Sprintf("**%s**\n\n%s", f.Title, f.Body)
		if f.Line > 0 {
			comments = append(comments, GitHubComment{Path: f.File, Line: f.Line, Side: "RIGHT", Body: text})
			continue
		}
		if fileLevel == 0 {
			body.WriteString("\n\n### Comentários de arquivo")
		}
		fileLevel++
		fmt.Fprintf(&body, "\n\n`%s` · %s", f.File, text)
	}

	return GitHubReview{
		CommitID: r.HeadSHA,
		Event:    githubEvents[r.Decision],
		Body:     strings.TrimSpace(body.String()),
		Comments: comments,
	}, nil
}
