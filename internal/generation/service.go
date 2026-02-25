package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"mini-atoms/internal/spec"
	"mini-atoms/internal/store"
)

const (
	ChatRoleUser      = store.ChatRoleUser
	ChatRoleAssistant = store.ChatRoleAssistant
	ChatRoleSystem    = store.ChatRoleSystem

	MaxRepairRetries = 1
)

type Client interface {
	GenerateSpecJSON(ctx context.Context, req ClientRequest) (string, error)
}

type ClientRequest struct {
	UserPrompt       string
	CurrentDraftJSON string
	RepairError      string
}

type ServiceDeps struct {
	Projects *store.ProjectRepo
	Chats    *store.ChatRepo
	Client   Client
}

type Service struct {
	projects *store.ProjectRepo
	chats    *store.ChatRepo
	client   Client
}

type GenerateDraftInput struct {
	UserID     int64
	ProjectID  int64
	UserPrompt string
}

type GenerateDraftResult struct {
	ProjectID        int64
	RoundNo          int
	DraftSpecJSON    string
	AssistantSummary string
}

func NewService(deps ServiceDeps) *Service {
	return &Service{
		projects: deps.Projects,
		chats:    deps.Chats,
		client:   deps.Client,
	}
}

func (s *Service) GenerateDraft(ctx context.Context, in GenerateDraftInput) (GenerateDraftResult, error) {
	if s == nil || s.projects == nil || s.chats == nil || s.client == nil {
		return GenerateDraftResult{}, fmt.Errorf("generation service not configured")
	}
	in.UserPrompt = strings.TrimSpace(in.UserPrompt)
	if in.UserID == 0 {
		return GenerateDraftResult{}, fmt.Errorf("generate draft: user id is required")
	}
	if in.ProjectID == 0 {
		return GenerateDraftResult{}, fmt.Errorf("generate draft: project id is required")
	}
	if in.UserPrompt == "" {
		return GenerateDraftResult{}, fmt.Errorf("generate draft: user prompt is required")
	}

	project, err := s.projects.GetProjectByUserAndID(ctx, in.UserID, in.ProjectID)
	if err != nil {
		return GenerateDraftResult{}, fmt.Errorf("generate draft load project: %w", err)
	}
	log.Printf("generation service start: project_id=%d user_id=%d prompt_chars=%d has_existing_draft=%t", project.ID, in.UserID, len([]rune(in.UserPrompt)), strings.TrimSpace(project.DraftSpecJSON) != "")

	roundNo, err := s.chats.NextRoundNo(ctx, project.ID)
	if err != nil {
		return GenerateDraftResult{}, fmt.Errorf("generate draft next round: %w", err)
	}
	if _, err := s.chats.CreateMessage(ctx, project.ID, roundNo, ChatRoleUser, in.UserPrompt); err != nil {
		return GenerateDraftResult{}, fmt.Errorf("generate draft write user message: %w", err)
	}

	clientReq := ClientRequest{
		UserPrompt:       in.UserPrompt,
		CurrentDraftJSON: project.DraftSpecJSON,
	}

	specJSON, appSpec, genErr := s.generateAndValidate(ctx, clientReq)
	if genErr != nil {
		_, _ = s.chats.CreateMessage(ctx, project.ID, roundNo, ChatRoleSystem, "生成失败："+genErr.Error())
		return GenerateDraftResult{}, genErr
	}

	if err := s.projects.UpdateProjectSpecsByID(ctx, in.UserID, project.ID, specJSON, project.PublishedSpecJSON); err != nil {
		_, _ = s.chats.CreateMessage(ctx, project.ID, roundNo, ChatRoleSystem, "保存草稿失败："+err.Error())
		return GenerateDraftResult{}, fmt.Errorf("generate draft save project draft: %w", err)
	}

	assistantSummary := buildAssistantSummary(appSpec)
	if _, err := s.chats.CreateMessage(ctx, project.ID, roundNo, ChatRoleAssistant, assistantSummary); err != nil {
		return GenerateDraftResult{}, fmt.Errorf("generate draft write assistant message: %w", err)
	}
	log.Printf("generation service success: project_id=%d user_id=%d round_no=%d draft_bytes=%d", project.ID, in.UserID, roundNo, len(specJSON))

	return GenerateDraftResult{
		ProjectID:        project.ID,
		RoundNo:          roundNo,
		DraftSpecJSON:    specJSON,
		AssistantSummary: assistantSummary,
	}, nil
}

func (s *Service) generateAndValidate(ctx context.Context, req ClientRequest) (string, spec.AppSpec, error) {
	var lastErr error
	curReq := req
	for attempt := 0; attempt <= MaxRepairRetries; attempt++ {
		attemptNo := attempt + 1
		attemptStartedAt := time.Now()
		log.Printf("generation attempt started: attempt=%d/%d has_current_draft=%t has_repair_error=%t", attemptNo, MaxRepairRetries+1, strings.TrimSpace(curReq.CurrentDraftJSON) != "", strings.TrimSpace(curReq.RepairError) != "")
		raw, err := s.client.GenerateSpecJSON(ctx, curReq)
		if err != nil {
			lastErr = fmt.Errorf("call llm client: %w", err)
			log.Printf("generation attempt client error: attempt=%d/%d duration_ms=%d err=%v", attemptNo, MaxRepairRetries+1, time.Since(attemptStartedAt).Milliseconds(), lastErr)
		} else {
			var appSpec spec.AppSpec
			if err := json.Unmarshal([]byte(raw), &appSpec); err != nil {
				lastErr = fmt.Errorf("parse spec json: %w", err)
				log.Printf("generation attempt parse error: attempt=%d/%d duration_ms=%d err=%v", attemptNo, MaxRepairRetries+1, time.Since(attemptStartedAt).Milliseconds(), lastErr)
			} else if err := spec.ValidateAppSpec(appSpec); err != nil {
				lastErr = fmt.Errorf("validate spec: %w", err)
				log.Printf("generation attempt validation error: attempt=%d/%d duration_ms=%d err=%v", attemptNo, MaxRepairRetries+1, time.Since(attemptStartedAt).Milliseconds(), lastErr)
			} else {
				appSpec, err = mergeNonDestructiveSchema(req.CurrentDraftJSON, appSpec)
				if err != nil {
					lastErr = fmt.Errorf("merge non-destructive schema: %w", err)
					log.Printf("generation attempt merge error: attempt=%d/%d duration_ms=%d err=%v", attemptNo, MaxRepairRetries+1, time.Since(attemptStartedAt).Milliseconds(), lastErr)
					goto attemptDone
				}
				canonical, err := json.Marshal(appSpec)
				if err != nil {
					return "", spec.AppSpec{}, fmt.Errorf("marshal validated spec: %w", err)
				}
				log.Printf("generation attempt succeeded: attempt=%d/%d duration_ms=%d", attemptNo, MaxRepairRetries+1, time.Since(attemptStartedAt).Milliseconds())
				return string(canonical), appSpec, nil
			}
		}

	attemptDone:
		if attempt < MaxRepairRetries {
			log.Printf("generation attempt retrying: next_attempt=%d/%d", attemptNo+1, MaxRepairRetries+1)
			curReq.RepairError = lastErr.Error()
			continue
		}
	}

	return "", spec.AppSpec{}, fmt.Errorf("generate draft failed after repair retry: %w", lastErr)
}

func mergeNonDestructiveSchema(currentDraftJSON string, generated spec.AppSpec) (spec.AppSpec, error) {
	if strings.TrimSpace(currentDraftJSON) == "" {
		return generated, nil
	}

	var existing spec.AppSpec
	if err := json.Unmarshal([]byte(currentDraftJSON), &existing); err != nil {
		return spec.AppSpec{}, fmt.Errorf("parse current draft json: %w", err)
	}
	if err := spec.ValidateAppSpec(existing); err != nil {
		return spec.AppSpec{}, fmt.Errorf("validate current draft spec: %w", err)
	}

	out := generated
	newCollectionIdx := make(map[string]int, len(out.Collections))
	for i, c := range out.Collections {
		newCollectionIdx[c.Name] = i
	}

	for _, oldCollection := range existing.Collections {
		idx, ok := newCollectionIdx[oldCollection.Name]
		if !ok {
			out.Collections = append(out.Collections, oldCollection)
			newCollectionIdx[oldCollection.Name] = len(out.Collections) - 1
			continue
		}

		merged := out.Collections[idx]
		newFieldNames := make(map[string]struct{}, len(merged.Fields))
		for _, f := range merged.Fields {
			newFieldNames[f.Name] = struct{}{}
		}
		for _, oldField := range oldCollection.Fields {
			if _, exists := newFieldNames[oldField.Name]; exists {
				continue
			}
			merged.Fields = append(merged.Fields, oldField)
			newFieldNames[oldField.Name] = struct{}{}
		}
		out.Collections[idx] = merged
	}

	if err := spec.ValidateAppSpec(out); err != nil {
		return spec.AppSpec{}, fmt.Errorf("validate merged spec: %w", err)
	}
	return out, nil
}

func buildAssistantSummary(appSpec spec.AppSpec) string {
	collectionCount := len(appSpec.Collections)
	pageCount := len(appSpec.Pages)
	blockCount := 0
	for _, p := range appSpec.Pages {
		blockCount += len(p.Blocks)
	}
	appName := strings.TrimSpace(appSpec.AppName)
	if appName == "" {
		appName = "未命名应用"
	}
	return fmt.Sprintf("已生成草稿《%s》，包含 %d 个数据集合、%d 个页面、%d 个区块。", appName, collectionCount, pageCount, blockCount)
}
