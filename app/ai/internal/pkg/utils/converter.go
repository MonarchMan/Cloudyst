package utils

import (
	"ai/ent"
	"ai/internal/biz/types"
	aipb "api/api/ai/common/v1"
	commonpb "api/api/common/v1"
	"encoding/json"
	"entmodule"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// 不建议使用jinzhu/copier进行快速属性复制，因为ent生成的proto文件里的DO不包含ent go代码里的Edges，关键就在这点，
// 且由于proto生成的go代码里无法携带json等标签，无法设置对应关系，导致需要额外定义一些转换规则，十分麻烦
// 所以这里手动复制属性，避免因为无法复制Edges而导致的问题
func EntApiKeyToProto(apiKey *ent.AiApiKey) *aipb.AiApiKey {
	if apiKey == nil {
		return nil
	}
	protoApiKey := &aipb.AiApiKey{
		Id:        int64(apiKey.ID),
		CreatedAt: timestamppb.New(apiKey.CreatedAt),
		UpdatedAt: timestamppb.New(apiKey.UpdatedAt),
		DeletedAt: nil,
		Name:      apiKey.Name,
		ApiKey:    apiKey.APIKey,
		Platform:  apiKey.Platform,
		Url:       apiKey.URL,
		Status:    StatusToProto[commonpb.Status](apiKey.Status),
	}

	if protoApiKey.DeletedAt != nil {
		protoApiKey.DeletedAt = timestamppb.New(*apiKey.DeletedAt)
	}

	if apiKey.Edges.AiModel != nil {
		protoApiKey.AiModel = lo.Map(apiKey.Edges.AiModel, func(item *ent.AiModel, index int) *aipb.AiModel {
			return EntModelToProto(item)
		})
	}

	return protoApiKey
}

func ProtoApiKeyToEnt(apiKey *aipb.AiApiKey) *ent.AiApiKey {
	if apiKey == nil {
		return nil
	}
	protoApiKey := &ent.AiApiKey{
		ID:        int(apiKey.Id),
		CreatedAt: apiKey.CreatedAt.AsTime(),
		UpdatedAt: apiKey.UpdatedAt.AsTime(),
		DeletedAt: nil,
		Name:      apiKey.Name,
		APIKey:    apiKey.ApiKey,
		Platform:  apiKey.Platform,
		URL:       apiKey.Url,
		Status:    entmodule.StatusFromProto(apiKey.Status),
	}
	if protoApiKey.DeletedAt != nil {
		deletedAt := apiKey.DeletedAt.AsTime()
		protoApiKey.DeletedAt = &deletedAt
	}

	return protoApiKey
}

func EntModelToProto(model *ent.AiModel) *aipb.AiModel {
	if model == nil {
		return nil
	}
	protoModel := &aipb.AiModel{
		Id:          int64(model.ID),
		CreatedAt:   timestamppb.New(model.CreatedAt),
		UpdatedAt:   timestamppb.New(model.UpdatedAt),
		Name:        model.Name,
		Type:        model.Type,
		Platform:    model.Platform,
		Sort:        int64(model.Sort),
		Status:      StatusToProto[commonpb.Status](model.Status),
		Temperature: model.Temperature,
		MaxTokens:   int64(model.MaxTokens),
		MaxContext:  int64(model.MaxContext),
	}

	if protoModel.DeletedAt != nil {
		protoModel.DeletedAt = timestamppb.New(*model.DeletedAt)
	}

	if model.Edges.AiAPIKey != nil {
		protoModel.AiApiKey = EntApiKeyToProto(model.Edges.AiAPIKey)
	}

	return protoModel
}

func ProtoModelToEnt(model *aipb.AiModel) *ent.AiModel {
	if model == nil {
		return nil
	}
	entModel := &ent.AiModel{
		ID:          int(model.Id),
		CreatedAt:   model.CreatedAt.AsTime(),
		UpdatedAt:   model.UpdatedAt.AsTime(),
		Name:        model.Name,
		Type:        model.Type,
		Platform:    model.Platform,
		Sort:        int(model.Sort),
		Status:      entmodule.StatusFromProto(model.Status),
		Temperature: model.Temperature,
		MaxTokens:   int(model.MaxTokens),
		MaxContext:  int(model.MaxContext),
		KeyID:       int(model.KeyId),
		Edges:       ent.AiModelEdges{},
	}

	if model.DeletedAt != nil {
		deletedAt := model.DeletedAt.AsTime()
		entModel.DeletedAt = &deletedAt
	}

	if model.AiApiKey != nil {
		entModel.Edges.AiAPIKey = ProtoApiKeyToEnt(model.AiApiKey)
	}

	return entModel
}

func EntToolToProto(tool *ent.AiTool) *aipb.AiTool {
	if tool == nil {
		return nil
	}
	protoTool := &aipb.AiTool{
		Id:          int64(tool.ID),
		CreatedAt:   timestamppb.New(tool.CreatedAt),
		UpdatedAt:   timestamppb.New(tool.UpdatedAt),
		Name:        tool.Name,
		Description: tool.Description,
		Type:        tool.Type,
		Parameters:  tool.Parameters,
		Status:      StatusToProto[commonpb.Status](tool.Status),
	}
	if tool.DeletedAt != nil {
		protoTool.DeletedAt = timestamppb.New(*tool.DeletedAt)
	}
	return protoTool
}

func ProtoToolToEnt(tool *aipb.AiTool) *ent.AiTool {
	if tool == nil {
		return nil
	}
	entTool := &ent.AiTool{
		ID:          int(tool.Id),
		CreatedAt:   tool.CreatedAt.AsTime(),
		UpdatedAt:   tool.UpdatedAt.AsTime(),
		Name:        tool.Name,
		Description: tool.Description,
		Type:        tool.Type,
		Parameters:  tool.Parameters,
		Status:      entmodule.StatusFromProto(tool.Status),
	}
	if tool.DeletedAt != nil {
		deletedAt := tool.DeletedAt.AsTime()
		entTool.DeletedAt = &deletedAt
	}
	return entTool
}

func EntConversationToProto(conversation *ent.AiChatConversation) *aipb.AiChatConversation {
	if conversation == nil {
		return nil
	}
	protoConversation := &aipb.AiChatConversation{
		Id:            int64(conversation.ID),
		CreatedAt:     timestamppb.New(conversation.CreatedAt),
		UpdatedAt:     timestamppb.New(conversation.UpdatedAt),
		Title:         conversation.Title,
		Pinned:        conversation.Pinned,
		UserId:        int64(conversation.UserID),
		RoleId:        int64(conversation.RoleID),
		SystemMessage: conversation.SystemMessage,
		ModelId:       int64(conversation.ModelID),
		Model:         conversation.Model,
		Temperature:   conversation.Temperature,
		MaxTokens:     int64(conversation.MaxTokens),
		MaxContexts:   int64(conversation.MaxContexts),
	}
	if conversation.DeletedAt != nil {
		protoConversation.DeletedAt = timestamppb.New(*conversation.DeletedAt)
	}
	return protoConversation
}

func ProtoConversationToEnt(conversation *aipb.AiChatConversation) *ent.AiChatConversation {
	if conversation == nil {
		return nil
	}
	entConversation := &ent.AiChatConversation{
		ID:            int(conversation.Id),
		CreatedAt:     conversation.CreatedAt.AsTime(),
		UpdatedAt:     conversation.UpdatedAt.AsTime(),
		Title:         conversation.Title,
		Pinned:        conversation.Pinned,
		UserID:        int(conversation.UserId),
		RoleID:        int(conversation.RoleId),
		SystemMessage: conversation.SystemMessage,
		ModelID:       int(conversation.ModelId),
		Model:         conversation.Model,
		Temperature:   conversation.Temperature,
		MaxTokens:     int(conversation.MaxTokens),
		MaxContexts:   int(conversation.MaxContexts),
	}
	if conversation.DeletedAt != nil {
		deletedAt := conversation.DeletedAt.AsTime()
		entConversation.DeletedAt = &deletedAt
	}

	return entConversation
}

func EntMessageToProto(message *ent.AiChatMessage) *aipb.AiChatMessage {
	if message == nil {
		return nil
	}
	protoMessage := &aipb.AiChatMessage{
		Id:             int64(message.ID),
		CreatedAt:      timestamppb.New(message.CreatedAt),
		UpdatedAt:      timestamppb.New(message.UpdatedAt),
		ConversationId: int64(message.ConversationID),
		UserId:         int64(message.UserID),
		RoleId:         int64(message.RoleID),
		ModelId:        int64(message.ModelID),
		Content:        message.Content,
		ReasonContent:  message.ReasonContent,
		UseContext:     message.UseContext,
		SegmentIds: lo.Map(message.SegmentIds, func(item int, _ int) int64 {
			return int64(item)
		}),
		AttachmentUrls: message.AttachmentUrls,
	}
	if message.DeletedAt != nil {
		protoMessage.DeletedAt = timestamppb.New(*message.DeletedAt)
	}
	if message.Edges.AiWebPage != nil {
		protoMessage.AiWebPage = lo.Map(message.Edges.AiWebPage, func(item *ent.AiWebPage, index int) *aipb.AiWebPage {
			return EntWebPageToProto(item)
		})
	}
	return protoMessage
}

func ProtoMessageToEnt(message *aipb.AiChatMessage) *ent.AiChatMessage {
	if message == nil {
		return nil
	}
	protoMessage := &ent.AiChatMessage{
		ID:             int(message.Id),
		CreatedAt:      message.CreatedAt.AsTime(),
		UpdatedAt:      message.UpdatedAt.AsTime(),
		ConversationID: int(message.ConversationId),
		UserID:         int(message.UserId),
		RoleID:         int(message.RoleId),
		ModelID:        int(message.ModelId),
		Content:        message.Content,
		ReasonContent:  message.ReasonContent,
		UseContext:     message.UseContext,
		SegmentIds: lo.Map(message.SegmentIds, func(item int64, _ int) int {
			return int(item)
		}),
		AttachmentUrls: message.AttachmentUrls,
		Edges:          ent.AiChatMessageEdges{},
	}
	if message.DeletedAt != nil {
		deletedAt := message.DeletedAt.AsTime()
		protoMessage.DeletedAt = &deletedAt
	}

	if message.AiWebPage != nil {
		protoMessage.Edges.AiWebPage = lo.Map(message.AiWebPage, func(item *aipb.AiWebPage, index int) *ent.AiWebPage {
			return ProtoWebPageToEnt(item)
		})
	}
	return protoMessage
}

func EntWebPageToProto(page *ent.AiWebPage) *aipb.AiWebPage {
	if page == nil {
		return nil
	}
	protoEdge := &aipb.AiWebPage{
		Id:        int64(page.ID),
		CreatedAt: timestamppb.New(page.CreatedAt),
		UpdatedAt: timestamppb.New(page.UpdatedAt),
		Name:      page.Name,
		Icon:      page.Icon,
		Title:     page.Title,
		Snippet:   page.Snippet,
		Summary:   page.Summary,
		MessageId: int64(page.MessageID),
	}

	if page.DeletedAt != nil {
		protoEdge.DeletedAt = timestamppb.New(*page.DeletedAt)
	}

	if page.Edges.AiChatMessage != nil {
		protoEdge.AiChatMessage = EntMessageToProto(page.Edges.AiChatMessage)
	}

	return protoEdge
}

func ProtoWebPageToEnt(page *aipb.AiWebPage) *ent.AiWebPage {
	if page == nil {
		return nil
	}
	entEdge := &ent.AiWebPage{
		ID:        int(page.Id),
		CreatedAt: page.CreatedAt.AsTime(),
		UpdatedAt: page.UpdatedAt.AsTime(),
		Name:      page.Name,
		Icon:      page.Icon,
		Title:     page.Title,
		Snippet:   page.Snippet,
		Summary:   page.Summary,
		MessageID: int(page.MessageId),
		Edges:     ent.AiWebPageEdges{},
	}

	if page.DeletedAt != nil {
		deletedAt := page.DeletedAt.AsTime()
		entEdge.DeletedAt = &deletedAt
	}

	if page.AiChatMessage != nil {
		entEdge.Edges.AiChatMessage = ProtoMessageToEnt(page.AiChatMessage)
	}

	return entEdge
}

func EntRoleToProto(role *ent.AiChatRole) *aipb.AiChatRole {
	if role == nil {
		return nil
	}
	protoRole := &aipb.AiChatRole{
		Id:            int64(role.ID),
		CreatedAt:     timestamppb.New(role.CreatedAt),
		UpdatedAt:     timestamppb.New(role.UpdatedAt),
		Name:          role.Name,
		Avatar:        role.Avatar,
		Description:   role.Description,
		Sort:          int64(role.Sort),
		UserId:        int64(role.UserID),
		PublicStatus:  role.PublicStatus,
		Category:      role.Category,
		SystemMessage: role.SystemMessage,
		KnowledgeIds: lo.Map(role.KnowledgeIds, func(item int, _ int) int64 {
			return int64(item)
		}),
		ToolIds: lo.Map(role.ToolIds, func(item int, _ int) int64 {
			return int64(item)
		}),
		McpClientNames: role.McpClientNames,
		Status:         StatusToProto[commonpb.Status](role.Status),
	}

	if role.DeletedAt != nil {
		protoRole.DeletedAt = timestamppb.New(*role.DeletedAt)
	}

	return protoRole
}

func ProtoRoleToEnt(role *aipb.AiChatRole) *ent.AiChatRole {
	if role == nil {
		return nil
	}
	entRole := &ent.AiChatRole{
		ID:            int(role.Id),
		CreatedAt:     role.CreatedAt.AsTime(),
		UpdatedAt:     role.UpdatedAt.AsTime(),
		Name:          role.Name,
		Avatar:        role.Avatar,
		Description:   role.Description,
		Sort:          int(role.Sort),
		UserID:        int(role.UserId),
		PublicStatus:  role.PublicStatus,
		Category:      role.Category,
		SystemMessage: role.SystemMessage,
		KnowledgeIds: lo.Map(role.KnowledgeIds, func(item int64, _ int) int {
			return int(item)
		}),
		ToolIds: lo.Map(role.ToolIds, func(item int64, _ int) int {
			return int(item)
		}),
		McpClientNames: role.McpClientNames,
		Status:         entmodule.StatusFromProto(role.Status),
	}

	if role.DeletedAt != nil {
		deletedAt := role.DeletedAt.AsTime()
		entRole.DeletedAt = &deletedAt
	}

	return entRole
}

// EntImageToProto 将 ent.AiImage 转换为 aipb.AiImage
func EntImageToProto(image *ent.AiImage) *aipb.AiImage {
	if image == nil {
		return nil
	}
	protoImage := &aipb.AiImage{
		Id:        int64(image.ID),
		CreatedAt: timestamppb.New(image.CreatedAt),
		UpdatedAt: timestamppb.New(image.UpdatedAt),
		UserId:    int64(image.UserID),
		Platform:  image.Platform,
		ModelId:   int64(image.ModelID),
		Model:     image.Model,
		Prompt:    image.Prompt,
		Width:     int64(image.Width),
		Height:    int64(image.Height),
		Status:    entmodule.ToProto[aipb.AiImage_Status, types.ImageStatus](types.ImageStatusProtoValues, image.Status),
		PicUrl:    image.PicURL,
		TaskId:    image.TaskID,
		Buttons:   image.Buttons,
	}

	if image.DeletedAt != nil {
		protoImage.DeletedAt = timestamppb.New(*image.DeletedAt)
	}

	// 将 map[string]interface{} 转换为 JSON 字符串
	if image.Options != nil {
		optionsBytes, err := json.Marshal(image.Options)
		if err == nil {
			protoImage.Options = string(optionsBytes)
		}
	}

	return protoImage
}

func ProtoImageToEnt(image *aipb.AiImage) *ent.AiImage {
	if image == nil {
		return nil
	}
	entImage := &ent.AiImage{
		ID:        int(image.Id),
		CreatedAt: image.CreatedAt.AsTime(),
		UpdatedAt: image.UpdatedAt.AsTime(),
		UserID:    int(image.UserId),
		Platform:  image.Platform,
		ModelID:   int(image.ModelId),
		Model:     image.Model,
		Prompt:    image.Prompt,
		Width:     int(image.Width),
		Height:    int(image.Height),
		Status:    entmodule.FromProto[aipb.AiImage_Status, types.ImageStatus](types.ProtoImageStatusValues, image.Status),
		PicURL:    image.PicUrl,
		TaskID:    image.TaskId,
		Buttons:   image.Buttons,
	}

	if image.DeletedAt != nil {
		deletedAt := image.DeletedAt.AsTime()
		entImage.DeletedAt = &deletedAt
	}

	// 将 JSON 字符串转换为 map[string]interface{}
	if image.Options != "" {
		var options map[string]interface{}
		err := json.Unmarshal([]byte(image.Options), &options)
		if err == nil {
			entImage.Options = options
		}
	}

	return entImage
}

func EntKnowledgeToProto(knowledge *ent.AiKnowledge) *aipb.AiKnowledge {
	if knowledge == nil {
		return nil
	}
	protoKnowledge := &aipb.AiKnowledge{
		Id:                  int64(knowledge.ID),
		CreatedAt:           timestamppb.New(knowledge.CreatedAt),
		UpdatedAt:           timestamppb.New(knowledge.UpdatedAt),
		Name:                knowledge.Name,
		Description:         knowledge.Description,
		EmbeddingModelId:    int64(knowledge.EmbeddingModelID),
		EmbeddingModel:      knowledge.EmbeddingModel,
		TopK:                int64(knowledge.TopK),
		SimilarityThreshold: knowledge.SimilarityThreshold,
		Status:              StatusToProto[commonpb.Status](knowledge.Status),
	}

	if knowledge.DeletedAt != nil {
		protoKnowledge.DeletedAt = timestamppb.New(*knowledge.DeletedAt)
	}

	if knowledge.Edges.AiKnowledgeDocument != nil {
		protoKnowledge.AiKnowledgeDocument = lo.Map(knowledge.Edges.AiKnowledgeDocument, func(item *ent.AiKnowledgeDocument, index int) *aipb.AiKnowledgeDocument {
			return EntKnowledgeDocumentToProto(item)
		})
	}
	return protoKnowledge
}

func ProtoKnowledgeToEnt(knowledge *aipb.AiKnowledge) *ent.AiKnowledge {
	if knowledge == nil {
		return nil
	}
	entKnowledge := &ent.AiKnowledge{
		ID:                  int(knowledge.Id),
		CreatedAt:           knowledge.CreatedAt.AsTime(),
		UpdatedAt:           knowledge.UpdatedAt.AsTime(),
		Name:                knowledge.Name,
		Description:         knowledge.Description,
		EmbeddingModelID:    int(knowledge.EmbeddingModelId),
		EmbeddingModel:      knowledge.EmbeddingModel,
		TopK:                int(knowledge.TopK),
		SimilarityThreshold: knowledge.SimilarityThreshold,
		Status:              entmodule.StatusFromProto(knowledge.Status),
		Edges:               ent.AiKnowledgeEdges{},
	}

	if knowledge.DeletedAt != nil {
		deletedAt := knowledge.DeletedAt.AsTime()
		entKnowledge.DeletedAt = &deletedAt
	}

	if knowledge.AiKnowledgeDocument != nil {
		entKnowledge.Edges.AiKnowledgeDocument = lo.Map(knowledge.AiKnowledgeDocument, func(item *aipb.AiKnowledgeDocument, index int) *ent.AiKnowledgeDocument {
			return ProtoKnowledgeDocumentToEnt(item)
		})
	}

	return entKnowledge
}

func EntKnowledgeDocumentToProto(document *ent.AiKnowledgeDocument) *aipb.AiKnowledgeDocument {
	if document == nil {
		return nil
	}
	protoDocument := &aipb.AiKnowledgeDocument{
		Id:               int64(document.ID),
		CreatedAt:        timestamppb.New(document.CreatedAt),
		UpdatedAt:        timestamppb.New(document.UpdatedAt),
		KnowledgeId:      int64(document.KnowledgeID),
		Name:             document.Name,
		Url:              document.URL,
		ContentLength:    int64(document.ContentLength),
		Tokens:           int64(document.Tokens),
		SegmentMaxTokens: int64(document.SegmentMaxTokens),
		RetrievalCount:   int64(document.RetrievalCount),
		Status:           StatusToProto[commonpb.Status](document.Status),
	}

	if document.DeletedAt != nil {
		protoDocument.DeletedAt = timestamppb.New(*document.DeletedAt)
	}

	if document.Edges.AiKnowledge != nil {
		protoDocument.AiKnowledge = EntKnowledgeToProto(document.Edges.AiKnowledge)
	}

	return protoDocument
}

func ProtoKnowledgeDocumentToEnt(document *aipb.AiKnowledgeDocument) *ent.AiKnowledgeDocument {
	if document == nil {
		return nil
	}
	entDocument := &ent.AiKnowledgeDocument{
		ID:               int(document.Id),
		CreatedAt:        document.CreatedAt.AsTime(),
		UpdatedAt:        document.UpdatedAt.AsTime(),
		KnowledgeID:      int(document.KnowledgeId),
		Name:             document.Name,
		URL:              document.Url,
		ContentLength:    int(document.ContentLength),
		Tokens:           int(document.Tokens),
		SegmentMaxTokens: int(document.SegmentMaxTokens),
		RetrievalCount:   int(document.RetrievalCount),
		Status:           entmodule.StatusFromProto(document.Status),
		Edges:            ent.AiKnowledgeDocumentEdges{},
	}

	if document.DeletedAt != nil {
		deletedAt := document.DeletedAt.AsTime()
		entDocument.DeletedAt = &deletedAt
	}
	if document.AiKnowledge != nil {
		entDocument.Edges.AiKnowledge = ProtoKnowledgeToEnt(document.AiKnowledge)
	}

	return entDocument
}

func EntKnowledgeSegmentToProto(segment *ent.AiKnowledgeSegment) *aipb.AiKnowledgeSegment {
	if segment == nil {
		return nil
	}
	protoSegment := &aipb.AiKnowledgeSegment{
		Id:         int64(segment.ID),
		CreatedAt:  timestamppb.New(segment.CreatedAt),
		UpdatedAt:  timestamppb.New(segment.UpdatedAt),
		DocumentId: int64(segment.DocumentID),
		//Content:        segment.Content,
		ContentLength:  int64(segment.ContentLength),
		Tokens:         int64(segment.Tokens),
		VectorId:       segment.VectorID,
		RetrievalCount: int64(segment.RetrievalCount),
		Status:         StatusToProto[commonpb.Status](segment.Status),
	}

	if segment.DeletedAt != nil {
		protoSegment.DeletedAt = timestamppb.New(*segment.DeletedAt)
	}
	if segment.Edges.AiKnowledgeDocument != nil {
		protoSegment.AiKnowledgeDocument = EntKnowledgeDocumentToProto(segment.Edges.AiKnowledgeDocument)
	}

	return protoSegment
}

func ProtoKnowledgeSegmentToEnt(segment *aipb.AiKnowledgeSegment) *ent.AiKnowledgeSegment {
	if segment == nil {
		return nil
	}
	entSegment := &ent.AiKnowledgeSegment{
		ID:         int(segment.Id),
		CreatedAt:  segment.CreatedAt.AsTime(),
		UpdatedAt:  segment.UpdatedAt.AsTime(),
		DocumentID: int(segment.DocumentId),
		//Content:        segment.Content,
		ContentLength:  int(segment.ContentLength),
		Tokens:         int(segment.Tokens),
		VectorID:       segment.VectorId,
		RetrievalCount: int(segment.RetrievalCount),
		Status:         entmodule.StatusFromProto(segment.Status),
		Edges:          ent.AiKnowledgeSegmentEdges{},
	}

	if segment.DeletedAt != nil {
		deletedAt := segment.DeletedAt.AsTime()
		entSegment.DeletedAt = &deletedAt
	}
	if segment.AiKnowledgeDocument != nil {
		entSegment.Edges.AiKnowledgeDocument = ProtoKnowledgeDocumentToEnt(segment.AiKnowledgeDocument)
	}
	return entSegment
}

func StatusToProto[T ~int32](s entmodule.Status) T {
	return entmodule.ToProto[T, entmodule.Status](entmodule.StatusProtoValues, s)
}
