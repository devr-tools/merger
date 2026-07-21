package mutations

import (
	"context"

	"github.com/devr-tools/merger/internal/domain"
	"github.com/devr-tools/merger/pkg/identity"
)

type SignalExtractor interface {
	Name() string
	Supports(domain.ChangedFile) bool
	Extract(context.Context, domain.ChangedFile) ([]domain.MutationSignal, error)
}

type ContentLoader interface {
	Load(context.Context, string) ([]byte, error)
}

type Analyzer interface {
	Name() string
	Supports(domain.ChangedFile) bool
	Analyze(context.Context, AnalysisInput) ([]domain.Mutation, error)
}

type AnalysisRequest struct {
	Repo    domain.RepoRef
	Ref     string
	Files   []domain.ChangedFile
	Content ContentLoader
}

type AnalysisInput struct {
	Repo    domain.RepoRef
	Ref     string
	File    domain.ChangedFile
	Content []byte
}

type Engine interface {
	Classify(context.Context, AnalysisRequest) ([]domain.Mutation, error)
}

type RuleBasedEngine struct {
	rules      []Rule
	extractors []SignalExtractor
	analyzers  []Analyzer
}

func NewRuleBasedEngine(rules []Rule, extractors []SignalExtractor, analyzers []Analyzer) *RuleBasedEngine {
	return &RuleBasedEngine{rules: rules, extractors: extractors, analyzers: analyzers}
}

func DefaultEngine() *RuleBasedEngine {
	return NewRuleBasedEngine(DefaultRules(), DefaultExtractors(), DefaultAnalyzers())
}

func DefaultEngineWithExternal(analyzers []Analyzer) *RuleBasedEngine {
	all := append(DefaultAnalyzers(), analyzers...)
	return NewRuleBasedEngine(DefaultRules(), DefaultExtractors(), all)
}

func (e *RuleBasedEngine) Classify(ctx context.Context, request AnalysisRequest) ([]domain.Mutation, error) {
	index := make(map[domain.MutationKind]*domain.Mutation)

	for _, file := range request.Files {
		signals, err := e.extractSignals(ctx, file)
		if err != nil {
			return nil, err
		}

		for _, rule := range e.rules {
			if !rule.Matches(file.Path, signals) {
				continue
			}

			addMutation(index, domain.Mutation{
				ID:          identity.New("mut"),
				Kind:        rule.Kind,
				Severity:    rule.Severity,
				Confidence:  rule.Confidence,
				Title:       rule.Title,
				Description: rule.Description,
				Files:       []string{file.Path},
				Signals:     signals,
				Detector:    rule.Name,
			})
		}

		content, _ := loadContent(ctx, request.Content, file.Path)
		for _, analyzer := range e.analyzers {
			if !analyzer.Supports(file) {
				continue
			}

			mutations, err := analyzer.Analyze(ctx, AnalysisInput{
				Repo:    request.Repo,
				Ref:     request.Ref,
				File:    file,
				Content: content,
			})
			if err != nil {
				return nil, err
			}

			for _, mutation := range mutations {
				addMutation(index, mutation)
			}
		}
	}

	if len(index) == 0 && len(request.Files) > 0 {
		fallback := domain.Mutation{
			ID:         identity.New("mut"),
			Kind:       domain.MutationUnknown,
			Severity:   domain.SeverityLow,
			Confidence: 0.35,
			Title:      "unclassified change surface",
			Detector:   "fallback",
		}
		for _, file := range request.Files {
			fallback.Files = append(fallback.Files, file.Path)
		}
		addMutation(index, fallback)
	}

	mutations := make([]domain.Mutation, 0, len(index))
	for _, mutation := range index {
		mutations = append(mutations, *mutation)
	}

	return mutations, nil
}
