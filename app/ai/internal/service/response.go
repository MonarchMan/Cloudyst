package service

import (
	"ai/ent"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/knowledge/rag/task"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/utils"
	pbadmin "api/api/ai/admin/v1"
	pbchat "api/api/ai/chat/v1"
	pbimage "api/api/ai/image/v1"
	pbknowledge "api/api/ai/knowledge/v1"
	pb "api/api/ai/model/v1"
	pbrole "api/api/ai/role/v1"
	commonpb "api/api/common/v1"
	"api/external/data/common"
	"common/auth"
	"common/hashid"
	"encoding/json"

	"github.com/cloudwego/eino/schema"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildChatConversation(conversation *ent.AiChatConversation, hasher hashid.Encoder) *pbchat.GetChatConversationResponse {
	if conversation == nil {
		return nil
	}
	return &pbchat.GetChatConversationResponse{
		Id:            hashid.EncodeID(hasher, conversation.ID, hashid.ChatConversationID),
		Title:         conversation.Title,
		Pinned:        conversation.Pinned,
		SystemMessage: conversation.SystemMessage,
		Model:         conversation.Model,
		Temperature:   conversation.Temperature,
		MaxTokens:     int64(conversation.MaxTokens),
		MaxContexts:   int64(conversation.MaxContexts),
		CreatedAt:     timestamppb.New(conversation.CreatedAt),
	}
}

func buildUserChatMessage(hasher hashid.Encoder, msg *ent.AiChatMessage) *pbchat.MessageRecord {
	if msg == nil {
		return nil
	}
	return &pbchat.MessageRecord{
		Id:      hashid.EncodeID(hasher, msg.ID, hashid.ChatMessageID),
		Type:    msg.Type,
		Content: msg.Content,
		//ReasonContent: msg.ReasonContent,
		CreatedAt:      timestamppb.New(msg.CreatedAt),
		ConversationId: hashid.EncodeID(hasher, msg.ConversationID, hashid.ChatConversationID),
	}
}

// buildAiChatMessage 构建ai聊天消息记录, 当output为nil时（数据库查询历史消息）, 则使用msg.Content和msg.ReasonContent构建
func buildAiChatMessage(hasher hashid.Encoder, msg *ent.AiChatMessage, output *schema.Message, segs []*types.KnowledgeSegment, pages []*types.WebPage) *pbchat.MessageRecord {
	if msg == nil {
		return nil
	}
	record := &pbchat.MessageRecord{
		Id:   hashid.EncodeID(hasher, msg.ID, hashid.ChatMessageID),
		Type: msg.Type,
		Segments: lo.Map(segs, func(seg *types.KnowledgeSegment, index int) *pbchat.KnowledgeSegment {
			return buildKnowledgeSegment(hasher, seg)
		}),
		WebPages: lo.Map(pages, func(page *types.WebPage, index int) *pbchat.WebPage {
			return buildWebPage(page)
		}),
		ConversationId: hashid.EncodeID(hasher, msg.ConversationID, hashid.ChatConversationID),
	}
	if output != nil {
		record.Content = output.Content
		record.ReasonContent = output.ReasoningContent
	} else {
		record.Content = msg.Content
		record.ReasonContent = msg.ReasonContent
	}
	return record
}

func buildKnowledgeSegment(hasher hashid.Encoder, seg *types.KnowledgeSegment) *pbchat.KnowledgeSegment {
	if seg == nil {
		return nil
	}
	return &pbchat.KnowledgeSegment{
		Id:         hashid.EncodeID(hasher, seg.ID, hashid.KnowledgeSegmentID),
		Content:    seg.Content,
		DocumentId: hashid.EncodeID(hasher, seg.DocumentID, hashid.KnowledgeSegmentID),
		//DocumentName: seg.DocumentName,
	}
}

func buildWebPage(page *types.WebPage) *pbchat.WebPage {
	if page == nil {
		return nil
	}
	return &pbchat.WebPage{
		Name:    page.Name,
		Icon:    page.Icon,
		Title:   page.Title,
		Url:     page.URL,
		Snippet: page.Snippet,
		Summary: page.Summary,
	}
}

func buildRoleResponse(role *ent.AiChatRole, hasher hashid.Encoder) *pbrole.GetRoleResponse {
	if role == nil {
		return nil
	}
	kids := make([]string, len(role.KnowledgeIds))
	for _, id := range role.KnowledgeIds {
		kids = append(kids, hashid.EncodeID(hasher, id, hashid.KnowledgeID))
	}
	tids := make([]string, len(role.ToolIds))
	for _, id := range role.ToolIds {
		tids = append(tids, hashid.EncodeID(hasher, id, hashid.ToolID))
	}

	return &pbrole.GetRoleResponse{
		Id:             hashid.EncodeID(hasher, role.ID, hashid.RoleID),
		Name:           role.Name,
		Avatar:         role.Avatar,
		Description:    role.Description,
		Sort:           int32(role.Sort),
		IsPublic:       role.PublicStatus,
		Category:       role.Category,
		SystemMessage:  role.SystemMessage,
		KnowledgeIds:   kids,
		ToolIds:        tids,
		McpClientNames: role.McpClientNames,
		CreatedAt:      timestamppb.New(role.CreatedAt),
	}
}

func buildImageResponse(image *ent.AiImage, hasher hashid.Encoder) *pbimage.GetImageResponse {
	if image == nil {
		return nil
	}
	resp := &pbimage.GetImageResponse{
		Id:         hashid.EncodeID(hasher, image.ID, hashid.ImageID),
		UserId:     hashid.EncodeID(hasher, image.UserID, hashid.UserID),
		Platform:   image.Platform,
		Model:      image.Platform,
		Prompt:     image.Prompt,
		Width:      int64(image.Width),
		Height:     int64(image.Height),
		Status:     string(image.Status),
		PicUrl:     image.PicURL,
		CreatedAt:  timestamppb.New(image.CreatedAt),
		FinishedAt: timestamppb.New(image.UpdatedAt),
	}
	for k, v := range image.Options {
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil
		}
		resp.Options[k] = string(jsonBytes)
	}
	return resp
}

func buildGetKnowledgeResponse(hasher hashid.Encoder, knowledge *ent.AiKnowledge) *pbknowledge.GetKnowledgeResponse {
	if knowledge == nil {
		return nil
	}
	return &pbknowledge.GetKnowledgeResponse{
		Id:             hashid.EncodeID(hasher, knowledge.ID, hashid.KnowledgeID),
		Name:           knowledge.Name,
		Description:    knowledge.Description,
		EmbeddingModel: knowledge.EmbeddingModel,
		TopK:           int32(knowledge.TopK),
		CreatedAt:      timestamppb.New(knowledge.CreatedAt),
		UpdatedAt:      timestamppb.New(knowledge.UpdatedAt),
	}
}

func buildGetDocumentResponse(hasher hashid.Encoder, doc *ent.AiKnowledgeDocument) *pbknowledge.GetDocumentResponse {
	if doc == nil {
		return nil
	}
	return &pbknowledge.GetDocumentResponse{
		Id:               hashid.EncodeID(hasher, doc.ID, hashid.KnowledgeDocumentID),
		Name:             doc.Name,
		Url:              doc.URL,
		Version:          doc.Version,
		Size:             int64(doc.ContentLength),
		Tokens:           int64(doc.Tokens),
		SegmentMaxTokens: int64(doc.SegmentMaxTokens),
		Progress:         string(doc.Progress),
		Status:           utils.StatusToProto[commonpb.Status](doc.Status),
		CreatedAt:        timestamppb.New(doc.CreatedAt),
		UpdatedAt:        timestamppb.New(doc.UpdatedAt),
	}
}

func buildCreateDocumentResponse(hasher hashid.Encoder, res *knowledge.UpsertDocumentResult) *pbknowledge.UpsertDocumentResponse {
	if res == nil {
		return nil
	}
	resp := &pbknowledge.UpsertDocumentResponse{
		Document: buildGetDocumentResponse(hasher, res.Document),
	}
	if res.Task != nil {
		resp.TaskId = hashid.EncodeID(hasher, res.Task.ID(), hashid.TaskID)
	}
	return resp
}

func buildUpdateDocumentResponse(hasher hashid.Encoder, res *knowledge.ReindexDocumentResult) *pbknowledge.UpsertDocumentResponse {
	if res == nil {
		return nil
	}
	resp := &pbknowledge.UpsertDocumentResponse{
		Document: buildGetDocumentResponse(hasher, res.Document),
	}
	if res.Task != nil {
		resp.TaskId = hashid.EncodeID(hasher, res.Task.ID(), hashid.TaskID)
	}
	return resp
}

func buildReindexDocumentResponse(hasher hashid.Encoder, res *knowledge.ReindexDocumentResult) *pbknowledge.ReindexDocumentResponse {
	if res == nil {
		return nil
	}
	resp := &pbknowledge.ReindexDocumentResponse{
		Progress: buildGetDocumentProgressResponse(hasher, res.Document),
	}
	if res.Task != nil {
		resp.TaskId = hashid.EncodeID(hasher, res.Task.ID(), hashid.TaskID)
	}
	return resp
}

func buildBatchCreateDocumentResponse(hasher hashid.Encoder, docs []*ent.AiKnowledgeDocument, task *task.IngestTask) *pbknowledge.BatchCreateDocumentResponse {
	if len(docs) == 0 {
		return nil
	}
	return &pbknowledge.BatchCreateDocumentResponse{
		Documents: lo.Map(docs, func(doc *ent.AiKnowledgeDocument, index int) *pbknowledge.GetDocumentResponse {
			return buildGetDocumentResponse(hasher, doc)
		}),
		Total:  int64(len(docs)),
		TaskId: hashid.EncodeID(hasher, task.ID(), hashid.TaskID),
	}
}

func buildBatchReindexDocumentResponse(hasher hashid.Encoder, docs []*ent.AiKnowledgeDocument, task *task.ReindexTask) *pbknowledge.BatchReindexDocumentResponse {
	if len(docs) == 0 {
		return nil
	}
	return &pbknowledge.BatchReindexDocumentResponse{
		Progresses: lo.Map(docs, func(doc *ent.AiKnowledgeDocument, index int) *pbknowledge.GetDocumentProgressResponse {
			return buildGetDocumentProgressResponse(hasher, doc)
		}),
		Total:  int64(len(docs)),
		TaskId: hashid.EncodeID(hasher, task.ID(), hashid.TaskID),
	}
}

func buildGetDocumentProgressResponse(hasher hashid.Encoder, doc *ent.AiKnowledgeDocument) *pbknowledge.GetDocumentProgressResponse {
	if doc == nil {
		return nil
	}
	return &pbknowledge.GetDocumentProgressResponse{
		Id:       hashid.EncodeID(hasher, doc.ID, hashid.KnowledgeDocumentID),
		Name:     doc.Name,
		Progress: string(doc.Progress),
	}
}

func buildSearchResponse(hasher hashid.Encoder, segs []*types.KnowledgeSegment, docMap map[int]*ent.AiKnowledgeDocument) *pbknowledge.SearchResponse {
	return &pbknowledge.SearchResponse{
		Total: int64(len(segs)),
		Results: lo.Map(segs, func(doc *types.KnowledgeSegment, index int) *pbknowledge.SearchResult {
			return &pbknowledge.SearchResult{
				DocId:       hashid.EncodeID(hasher, doc.DocumentID, hashid.KnowledgeDocumentID),
				DocUri:      docMap[doc.DocumentID].URL,
				DocVersion:  docMap[doc.DocumentID].Version,
				KnowledgeId: hashid.EncodeID(hasher, doc.KnowledgeID, hashid.KnowledgeID),
				Content:     doc.Content,
				Size:        int64(doc.ContentLen),
				Score:       doc.Score,
			}
		}),
	}
}

func buildKnowledgeStatsResponse(stats *types.KnowledgeStats) *pbknowledge.KnowledgeStatsResponse {
	if stats == nil {
		return nil
	}
	return &pbknowledge.KnowledgeStatsResponse{
		DocumentCount: int32(stats.DocumentCount),
		Ready:         int32(stats.Ready),
		Processing:    int32(stats.Processing),
		Failed:        int32(stats.Failed),
		SuccessRate:   stats.SuccessRate,
		TotalTokens:   stats.TotalTokens,
	}
}

func buildGetModelResponse(hasher hashid.Encoder, model *ent.AiModel) *pb.GetModelResponse {
	if model == nil {
		return nil
	}
	return &pb.GetModelResponse{
		Id:          hashid.EncodeID(hasher, model.ID, hashid.ModelID),
		Name:        model.Name,
		Type:        model.Type,
		Platform:    model.Platform,
		Temperature: model.Temperature,
		MaxTokens:   int32(model.MaxTokens),
		MaxContexts: int32(model.MaxContext),
	}
}

func buildTaskListResponse(hasher hashid.Encoder, res *data.ListTaskResult) *commonpb.ListTasksResponse {
	return &commonpb.ListTasksResponse{
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
		Tasks: lo.Map(res.Tasks, func(task *ent.Task, index int) *commonpb.TaskResponse {
			return buildTaskResponse(hasher, task)
		}),
	}
}

func buildTaskResponse(hasher hashid.Encoder, task *ent.Task) *commonpb.TaskResponse {
	return &commonpb.TaskResponse{
		Status:    string(task.Status),
		CreatedAt: timestamppb.New(task.CreatedAt),
		UpdatedAt: timestamppb.New(task.UpdatedAt),
		Id:        hashid.EncodeTaskID(hasher, task.ID),
		Type:      task.Type,
		Error:     auth.RedactSensitiveValues(task.PublicState.Error),
		ErrorHistory: lo.Map(task.PublicState.ErrorHistory, func(s string, index int) string {
			return auth.RedactSensitiveValues(s)
		}),
		Duration:   int64(task.PublicState.ExecutedDuration),
		ResumeTime: task.PublicState.ResumeTime,
		RetryCount: int32(task.PublicState.RetryCount),
	}
}

func buildAdminUpsertDocumentResponse(doc *ent.AiKnowledgeDocument, taskId int) *pbadmin.UpsertDocumentResponse {
	return &pbadmin.UpsertDocumentResponse{
		Document: utils.EntKnowledgeDocumentToProto(doc),
		TaskId:   int64(taskId),
	}
}

func buildAdminListTasksResponse(hasher hashid.Encoder, res *data.ListTaskResult) *commonpb.ListTaskResponse {
	if res == nil {
		return nil
	}
	return &commonpb.ListTaskResponse{
		Tasks: lo.Map(res.Tasks, func(task *ent.Task, index int) *commonpb.GetTaskResponse {
			var (
				uid string
			)

			uid = hashid.EncodeUserID(hasher, task.UserID)

			return &commonpb.GetTaskResponse{
				Task:       utils.EntTaskToProto(task),
				TaskHashId: hashid.EncodeTaskID(hasher, task.ID),
				UserHashId: uid,
			}
		}),
		Pagination: common.PaginationResultsToProto(res.PaginationResults),
	}
}
