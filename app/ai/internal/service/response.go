package service

import (
	"ai/ent"
	"ai/internal/biz/types"
	"ai/internal/pkg/utils"
	pbchat "api/api/ai/chat/v1"
	pbimage "api/api/ai/image/v1"
	pbknowledge "api/api/ai/knowledge/v1"
	pbrole "api/api/ai/role/v1"
	commonpb "api/api/common/v1"
	"common/hashid"
	"encoding/json"

	"github.com/cloudwego/eino/schema"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func buildChatConversation(conversation *ent.AiChatConversation, hasher hashid.Encoder) *pbchat.GetChatConversationResponse {
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
	return &pbchat.MessageRecord{
		Id:      hashid.EncodeID(hasher, msg.ID, hashid.ChatMessageID),
		Type:    msg.Type,
		Content: msg.Content,
		//ReasonContent: msg.ReasonContent,
		CreatedAt: timestamppb.New(msg.CreatedAt),
	}
}

// buildAiChatMessage 构建ai聊天消息记录, 当output为nil时（数据库查询历史消息）, 则使用msg.Content和msg.ReasonContent构建
func buildAiChatMessage(hasher hashid.Encoder, msg *ent.AiChatMessage, output *schema.Message, segs []*types.KnowledgeSegment, pages []*types.WebPage) *pbchat.MessageRecord {
	record := &pbchat.MessageRecord{
		Id:   hashid.EncodeID(hasher, msg.ID, hashid.ChatMessageID),
		Type: msg.Type,
		Segments: lo.Map(segs, func(seg *types.KnowledgeSegment, index int) *pbchat.KnowledgeSegment {
			return buildKnowledgeSegment(hasher, seg)
		}),
		WebPages: lo.Map(pages, func(page *types.WebPage, index int) *pbchat.WebPage {
			return buildWebPage(page)
		}),
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
	return &pbchat.KnowledgeSegment{
		Id:         hashid.EncodeID(hasher, seg.ID, hashid.KnowledgeSegmentID),
		Content:    seg.Content,
		DocumentId: hashid.EncodeID(hasher, seg.DocumentID, hashid.KnowledgeSegmentID),
		//DocumentName: seg.DocumentName,
	}
}

func buildWebPage(page *types.WebPage) *pbchat.WebPage {
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
	return &pbknowledge.GetDocumentResponse{
		Id:               hashid.EncodeID(hasher, doc.ID, hashid.KnowledgeDocumentID),
		Name:             doc.Name,
		Url:              doc.URL,
		Size:             int64(doc.ContentLength),
		Tokens:           int64(doc.Tokens),
		SegmentMaxTokens: int64(doc.SegmentMaxTokens),
		Status:           utils.StatusToProto[commonpb.Status](doc.Status),
		CreatedAt:        timestamppb.New(doc.CreatedAt),
		UpdatedAt:        timestamppb.New(doc.UpdatedAt),
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
