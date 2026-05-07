package service

import (
	"ai/ent"
	"ai/internal/biz/chat"
	"ai/internal/biz/knowledge"
	"ai/internal/biz/knowledge/rag/retrieval"
	"ai/internal/biz/model"
	"ai/internal/biz/types"
	"ai/internal/data"
	"ai/internal/pkg/eino/tool/factory"
	pb "api/api/ai/chat/v1"
	commonpb "api/api/common/v1"
	"api/external/data/common"
	"api/external/trans"
	"common/hashid"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/samber/lo"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ChatService struct {
	pb.UnimplementedChatServer
	rs     *RoleService
	hasher hashid.Encoder
	cb     chat.ChatBiz
	kb     knowledge.KnowledgeBiz
	mb     model.ModelBiz
	l      *log.Helper
	wsm    chat.Searcher
	tr     *factory.ToolRegistry
}

type (
	invokeFN func(ctx context.Context, model emodel.ToolCallingChatModel, input []*schema.Message, sendRecord *pb.MessageRecord,
		onChunkFN onChunkFN, opts ...emodel.Option) (*schema.Message, error)
	onChunkFN func(chunk *schema.Message, response *pb.SendMessageResponse) error
)

func NewChatService() *ChatService {
	return &ChatService{}
}

func (s *ChatService) CreateChatConversation(ctx context.Context, req *pb.CreateChatConversationRequest) (*pb.GetChatConversationResponse, error) {
	roleID, err := validateID(s.hasher, req.RoleId, hashid.RoleID, true)
	if err != nil {
		return nil, err
	}

	modelID, err := validateID(s.hasher, req.ModelId, hashid.ModelID, true)
	if err != nil {
		return nil, err
	}
	knowledgeID, err := validateID(s.hasher, req.KnowledgeId, hashid.KnowledgeID, true)
	if err != nil {
		return nil, err
	}

	created, err := s.cb.CreateConversation(ctx, roleID, modelID, knowledgeID)
	return buildChatConversation(created, s.hasher), err
}
func (s *ChatService) UpdateChatConversation(ctx context.Context, req *pb.UpdateChatConversationRequest) (*pb.GetChatConversationResponse, error) {
	conversationID, err := validateID(s.hasher, req.Id, hashid.ChatConversationID, false)
	if err != nil {
		return nil, err
	}

	modelID, err := validateID(s.hasher, req.ModelId, hashid.ModelID, true)
	if err != nil {
		return nil, err
	}
	args := &chat.UpdateConversationArgs{
		ExistedID:   conversationID,
		Title:       req.Title,
		Pinned:      false,
		SysMsg:      req.SystemMessage,
		ModelID:     modelID,
		Temperature: req.Temperature,
		MaxTokens:   int(req.MaxTokens),
		MaxContexts: int(req.MaxContexts),
	}
	updated, err := s.cb.UpdateConversation(ctx, args)
	return buildChatConversation(updated, s.hasher), err
}
func (s *ChatService) ListChatConversationMe(ctx context.Context, req *emptypb.Empty) (*pb.GetMultiConversationsResponse, error) {
	conversations, err := s.cb.ListConversationByUser(ctx)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list conversations: %w", err)
	}
	resp := &pb.GetMultiConversationsResponse{
		Conversations: lo.Map(conversations, func(c *ent.AiChatConversation, index int) *pb.GetChatConversationResponse {
			return buildChatConversation(c, s.hasher)
		}),
	}
	return resp, nil
}
func (s *ChatService) GetChatConversation(ctx context.Context, req *pb.SimpleChatConversationRequest) (*pb.GetChatConversationResponse, error) {
	cid, err := validateID(s.hasher, req.Id, hashid.ChatConversationID, false)
	if err != nil {
		return nil, err
	}
	c, err := s.cb.GetConversation(ctx, cid)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get conversation: %w", err)
	}
	return buildChatConversation(c, s.hasher), nil
}
func (s *ChatService) DeleteChatConversation(ctx context.Context, req *pb.SimpleChatConversationRequest) (*emptypb.Empty, error) {
	cid, err := validateID(s.hasher, req.Id, hashid.ChatConversationID, false)
	if err != nil {
		return nil, err
	}
	err = s.cb.DeleteConversation(ctx, &chat.DeleteConversationArgs{ID: cid})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete conversation: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *ChatService) DeleteUnpinnedChatConversations(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	if err := s.cb.DeleteConversation(ctx, &chat.DeleteConversationArgs{Unpinned: true}); err != nil {
		return nil, commonpb.ErrorDb("delete unpinned conversations failed: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *ChatService) ListChatConversations(ctx context.Context, req *pb.ListConversationRequest) (*pb.ListConversationResponse, error) {
	u := trans.FromContext(ctx)
	// 构建分页参数
	args := &data.ListChatConversationArgs{
		PaginationArgs: common.ConvertPaginationArgs(req.Pagination),
		Title:          req.Title,
		UserID:         u.ID,
	}
	if req.Start != nil {
		args.Start = req.Start.AsTime()
	}
	if req.End != nil {
		args.End = req.End.AsTime()
	}
	conversations, err := s.cb.ListConversations(ctx, args)
	if err != nil {
		return nil, commonpb.ErrorDb("list conversations failed: %w", err)
	}
	return &pb.ListConversationResponse{
		Conversations: lo.Map(conversations.Conversations, func(c *ent.AiChatConversation, index int) *pb.GetChatConversationResponse {
			return buildChatConversation(c, s.hasher)
		}),
		Pagination: common.PaginationResultsToProto(conversations.PaginationResults),
	}, nil
}
func (s *ChatService) DeleteMessage(ctx context.Context, req *pb.DeleteMessageRequest) (*emptypb.Empty, error) {
	mid, err := validateID(s.hasher, req.Id, hashid.ChatMessageID, false)
	if err != nil {
		return nil, err
	}
	if err := s.cb.DeleteMessage(ctx, mid, req.Cascade); err != nil {
		return nil, commonpb.ErrorDb("Failed to delete message: %w", err)
	}
	return &emptypb.Empty{}, nil
}
func (s *ChatService) ListConversationMessage(ctx context.Context, req *pb.ListConversationMessagesRequest) (*pb.ListConversationMessagesResponse, error) {
	cid, err := validateID(s.hasher, req.ConversationId, hashid.ChatMessageID, false)
	if err != nil {
		return nil, err
	}
	messages, err := s.cb.ListConversationMessages(ctx, cid, common.ConvertPaginationArgs(req.Pagination))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list messages: %w", err)
	}
	//关联段落
	var sids []int
	for _, m := range messages.ChatMessages {
		sids = append(sids, m.SegmentIds...)
	}
	sids = lo.Uniq(sids)
	segments, err := s.kb.GetKnowledgeSegments(ctx, sids)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get knowledge segments: %w", err)
	}

	msgRecords := make([]*pb.MessageRecord, len(messages.ChatMessages))
	for i, m := range messages.ChatMessages {
		if m.Type == string(schema.User) {
			msgRecords[i] = buildUserChatMessage(s.hasher, m)
		} else {
			msgSegs := make([]*types.KnowledgeSegment, 0, len(m.SegmentIds))
			for _, sid := range m.SegmentIds {
				msgSegs = append(msgSegs, segments[sid])
			}
			msgRecords[i] = buildAiChatMessage(s.hasher, m, nil, msgSegs, messages.PageMap[m.ID])
		}
	}
	return &pb.ListConversationMessagesResponse{
		Messages:   msgRecords,
		Pagination: common.PaginationResultsToProto(messages.PaginationResults),
	}, nil
}

func (s *ChatService) StreamChatHandler(ctx khttp.Context) error {
	var req pb.SendMessageRequest
	if err := ctx.Bind(&req); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}

	onChunk, onFirst := s.HandlerChunkFn(ctx)
	if onChunk == nil || onFirst == nil {
		return fmt.Errorf("streaming unsupported")
	}
	return s.streamChat(ctx, "", req.ConversationId, req.ModelId, req.Content, req.UseContext, req.UseSearch,
		req.AttachmentUrls, onChunk, onFirst)
}

func (s *ChatService) streamChat(ctx context.Context, rawMsgID, rawConversationID, rawModelID, content string, useContext, useSearch bool,
	attachmentUrls []string, onChunk chat.OnChunkFN, onFirstFN chat.OnFirstChunkFN) error {
	// 1.1 校验消息id是否有效
	msgID, err := validateID(s.hasher, rawMsgID, hashid.ChatMessageID, true)
	if err != nil {
		return err
	}
	// 1.2 校验对话id是否有效
	cid, err := validateID(s.hasher, rawConversationID, hashid.ChatConversationID, true)
	if err != nil {
		return err
	}

	// 1.3 校验模型id是否有效
	mid, err := validateID(s.hasher, rawModelID, hashid.ModelID, false)
	if err != nil {
		return err
	}

	// 2. 获取模型
	m, err := s.mb.GetActiveModel(ctx, mid)
	if err != nil {
		return err
	}
	sendArgs := &chat.SendMessageArgs{
		MsgID:          msgID,
		ConversationID: cid,
		Content:        content,
		UseContext:     useContext,
		UseSearch:      useSearch,
		AttachmentUrls: attachmentUrls,
	}
	// 3. 获取知识库检索工具
	var rt tool.InvokableTool
	if rtFn, ok := s.tr.GetFactory(retrieval.RetrievalToolName); ok {
		rt, err = rtFn(nil)
		if err != nil {
			return err
		}
	}

	// 4. 流式调用llm
	_, err = s.cb.Stream(ctx, sendArgs, m, rt, onChunk, onFirstFN)
	if err != nil {
		return commonpb.ErrorInternalSetting("Failed to send stream message: %w", err)
	}
	return nil
}

func (s *ChatService) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	mid, err := validateID(s.hasher, req.ModelId, hashid.ModelID, false)
	if err != nil {
		return nil, err
	}
	m, err := s.mb.GetActiveModel(ctx, mid)
	if err != nil {
		return nil, err
	}
	// 1 校验对话id是否有效
	cid, err := validateID(s.hasher, req.ConversationId, hashid.ChatConversationID, false)
	if err != nil {
		return nil, err
	}
	sendArgs := &chat.SendMessageArgs{
		ConversationID: cid,
		Content:        req.Content,
		UseContext:     req.UseContext,
		UseSearch:      req.UseSearch,
		AttachmentUrls: req.AttachmentUrls,
	}

	record, err := s.cb.Generate(ctx, sendArgs, m, nil)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to send message: %w", err)
	}
	sendRecord := buildUserChatMessage(s.hasher, record.UserMsg)
	assistMsg := buildAiChatMessage(s.hasher, record.AiMsg, record.Output, record.InnerRecord.Segs, record.InnerRecord.WebSearch.WebPages)
	return &pb.SendMessageResponse{
		Send:    sendRecord,
		Receive: assistMsg,
	}, nil
}

func (s *ChatService) RetryMessage(ctx context.Context, req *pb.RetryMessageRequest) (*pb.SendMessageResponse, error) {
	// 1.1 校验消息id是否有效
	msgID, err := validateID(s.hasher, req.Id, hashid.ChatMessageID, false)
	if err != nil {
		return nil, err
	}
	// 1.2 校验模型id是否有效
	mid, err := validateID(s.hasher, req.ModelId, hashid.ModelID, false)
	if err != nil {
		return nil, err
	}
	m, err := s.mb.GetActiveModel(ctx, mid)
	if err != nil {
		return nil, err
	}

	// 2. 构建发送参数，重新发送消息
	sendArgs := &chat.SendMessageArgs{
		MsgID:      msgID,
		UseContext: req.UseContext,
		UseSearch:  req.UseSearch,
	}

	record, err := s.cb.Generate(ctx, sendArgs, m, nil)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to send message: %w", err)
	}
	sendRecord := buildUserChatMessage(s.hasher, record.UserMsg)
	assistMsg := buildAiChatMessage(s.hasher, record.AiMsg, record.Output, record.InnerRecord.Segs, record.InnerRecord.WebSearch.WebPages)
	return &pb.SendMessageResponse{
		Send:    sendRecord,
		Receive: assistMsg,
	}, nil
}

func (s *ChatService) SendMessageStream(req *pb.SendMessageRequest, grpcStream grpc.ServerStreamingServer[pb.SendMessageResponse]) error {
	onChunk, onFirst := s.streamChunkFn(grpcStream)

	return s.streamChat(grpcStream.Context(), "", req.ConversationId, req.ModelId, req.Content, req.UseContext, req.UseSearch,
		req.AttachmentUrls, onChunk, onFirst)
}
func (s *ChatService) RetryMessageStream(req *pb.RetryMessageRequest, grpcStream grpc.ServerStreamingServer[pb.SendMessageResponse]) error {
	onChunk, onFirst := s.streamChunkFn(grpcStream)

	return s.streamChat(grpcStream.Context(), req.Id, "", req.ModelId, "", req.UseContext, req.UseSearch,
		nil, onChunk, onFirst)
}
func (s *ChatService) StreamRetryChatHandler(ctx khttp.Context) error {
	var req pb.RetryMessageRequest
	if err := ctx.Bind(&req); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}
	onChunk, onFirst := s.HandlerChunkFn(ctx)
	if onChunk == nil || onFirst == nil {
		return fmt.Errorf("streaming unsupported")
	}

	return s.streamChat(ctx, req.Id, "", req.ModelId, "", req.UseContext, req.UseSearch,
		nil, onChunk, onFirst)
}
func (s *ChatService) PatchMessage(ctx context.Context, req *pb.PatchMessageRequest) (*pb.SendMessageResponse, error) {
	msgID, err := validateID(s.hasher, req.Id, hashid.ChatMessageID, false)
	if err != nil {
		return nil, err
	}
	mid, err := validateID(s.hasher, req.ModelId, hashid.ModelID, false)
	if err != nil {
		return nil, err
	}
	m, err := s.mb.GetActiveModel(ctx, mid)
	if err != nil {
		return nil, err
	}
	sendArgs := &chat.SendMessageArgs{
		MsgID:      msgID,
		Content:    req.Content,
		UseContext: req.UseContext,
		UseSearch:  req.UseSearch,
	}

	record, err := s.cb.Generate(ctx, sendArgs, m, nil)
	if err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to send message: %w", err)
	}
	sendRecord := buildUserChatMessage(s.hasher, record.UserMsg)
	assistMsg := buildAiChatMessage(s.hasher, record.AiMsg, record.Output, record.InnerRecord.Segs, record.InnerRecord.WebSearch.WebPages)
	return &pb.SendMessageResponse{
		Send:    sendRecord,
		Receive: assistMsg,
	}, nil
}
func (s *ChatService) PatchMessageHandler(ctx khttp.Context) error {
	var req pb.PatchMessageRequest
	if err := ctx.Bind(&req); err != nil {
		return fmt.Errorf("failed to bind request body: %w", err)
	}
	onChunk, onFirst := s.HandlerChunkFn(ctx)
	if onChunk == nil || onFirst == nil {
		return fmt.Errorf("streaming unsupported")
	}
	return s.streamChat(ctx, req.Id, "", req.ModelId, req.Content, req.UseContext, req.UseSearch,
		nil, onChunk, onFirst)
}
func validateID(hasher hashid.Encoder, rawID string, idType int, canEmpty bool) (int, error) {
	if strings.TrimSpace(rawID) == "" {
		if !canEmpty {
			return 0, commonpb.ErrorParamInvalid("invalid id %s", rawID)
		}
		return 0, nil
	}
	id, err := hasher.Decode(rawID, idType)
	if err != nil {
		return 0, commonpb.ErrorParamInvalid("invalid id %s", rawID)
	}
	return id, nil
}
func (s *ChatService) streamChunkFn(grpcStream grpc.ServerStreamingServer[pb.SendMessageResponse]) (chat.OnChunkFN, chat.OnFirstChunkFN) {
	// stream 调用过程中每个chunk的处理方法（不包含第一个）
	onChunk := func(chunk *schema.Message, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error {
		resp := &pb.SendMessageResponse{
			Receive: buildAiChatMessage(s.hasher, aiMsg, chunk, record.Segs, record.WebSearch.WebPages),
		}
		return grpcStream.Send(resp)
	}

	// stream 调用过程中第一个chunk的处理方法
	onFirst := func(chunk *schema.Message, userMsg, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error {
		resp := &pb.SendMessageResponse{
			Send:    buildUserChatMessage(s.hasher, userMsg),
			Receive: buildAiChatMessage(s.hasher, aiMsg, chunk, record.Segs, record.WebSearch.WebPages),
		}
		return grpcStream.Send(resp)
	}
	return onChunk, onFirst
}

func (s *ChatService) HandlerChunkFn(ctx khttp.Context) (chat.OnChunkFN, chat.OnFirstChunkFN) {
	w := ctx.Response()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, nil
	}

	onChunk := func(chunk *schema.Message, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error {
		resp := buildAiChatMessage(s.hasher, aiMsg, chunk, record.Segs, record.WebSearch.WebPages)
		bytes, _ := json.Marshal(resp)
		_, err := fmt.Fprintf(w, "data: %s\n\n", bytes)
		flusher.Flush()
		return err
	}
	onFirstChunk := func(chunk *schema.Message, userMsg, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error {
		resp := &pb.SendMessageResponse{
			Send:    buildUserChatMessage(s.hasher, userMsg),
			Receive: buildAiChatMessage(s.hasher, aiMsg, chunk, record.Segs, record.WebSearch.WebPages),
		}
		bytes, _ := json.Marshal(resp)
		_, err := fmt.Fprintf(w, "data: %s\n\n", bytes)
		flusher.Flush()
		return err
	}
	return onChunk, onFirstChunk
}
