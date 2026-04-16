package chat

import (
	"ai/ent"
	"ai/ent/aimodel"
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"ai/internal/data/vector"
	"ai/internal/pkg/eino/message"
	"ai/internal/pkg/eino/tool/factory"
	aimcp "ai/internal/pkg/mcp"
	commonpb "api/api/common/v1"
	"api/external/trans"
	"common/db"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino-ext/components/document/loader/url"
	"github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/document"
	emodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/samber/lo"
	"github.com/samber/lo/mutable"
)

const (
	defaultTitle   = "新对话"
	webSearchCount = 10
)

type (
	ChatBiz interface {
		CreateConversation(ctx context.Context, roleID int, modelID int, knowledgeID int) (*ent.AiChatConversation, error)
		UpdateConversation(ctx context.Context, args *UpdateConversationArgs) (*ent.AiChatConversation, error)
		ListConversationByUser(ctx context.Context) ([]*ent.AiChatConversation, error)
		GetConversation(ctx context.Context, id int) (*ent.AiChatConversation, error)
		DeleteConversation(ctx context.Context, args *DeleteConversationArgs) error
		PageConversation(ctx context.Context, args *data.ListChatConversationArgs) (*data.ListChatConversationResult, error)

		PrepareTemplate(ctx context.Context, args *SendMessageArgs) (map[string]any, *ent.AiChatMessage, error)
		PrepareInput(ctx context.Context, args *SendMessageArgs) (*InputInfo, error)
		BuildInput(info *InputInfo) []*schema.Message
		GetToolInfosByRoleID(info *types.RoleInfo) ([]*schema.ToolInfo, error)
		PageConversationMessage(ctx context.Context, conversationID int, pagination *db.PaginationArgs) (*data.ListChatMessageResult, error)
		DeleteMessage(ctx context.Context, id int) error
		DeleteMessagesByConversationID(ctx context.Context, conversationID int) error
		ListMessagesByConversationID(ctx context.Context, conversationID int, pagination *db.PaginationArgs) (*data.ListChatMessageResult, error)
		SaveMessage(ctx context.Context, args *data.CreateChatMessageArgs) (*ent.AiChatMessage, error)
		GetRoleInfo(ctx context.Context, roleID int) (*types.RoleInfo, error)
		BuildTools(retrieve tool.InvokableTool, info *types.RoleInfo, useSearch bool, useRag bool) ([]tool.BaseTool, error)
		Generate(ctx context.Context, args *SendMessageArgs, m emodel.ToolCallingChatModel,
			retrieveTool tool.InvokableTool) (*ChatRecord, error)
		Stream(ctx context.Context, args *SendMessageArgs, m emodel.ToolCallingChatModel, retrieveTool tool.InvokableTool,
			onChunk OnChunkFN, onFirst OnFirstChunkFN) (*ChatRecord, error)
	}

	chatBiz struct {
		cc        data.ChatConversationClient
		rc        data.RoleClient
		kc        data.KnowledgeClient
		mc        data.ModelClient
		cmc       data.ChatMessageClient
		wc        data.WebPageClient
		tc        data.ToolClient
		vs        vector.VectorStore
		urlReader url.Loader
		l         *log.Helper
		mcm       aimcp.MCPClientManager
		wsm       *Searcher
		conf      *conf.Bootstrap
		tr        *factory.ToolRegistry
	}

	UpdateConversationArgs struct {
		ExistedID   int
		Title       string
		Pinned      bool
		SysMsg      string
		ModelID     int
		Temperature float64
		MaxTokens   int
		MaxContexts int
	}

	DeleteConversationArgs struct {
		ID       int
		Unpinned bool
	}

	SendMessageArgs struct {
		ConversationID int
		Content        string
		UseContext     bool
		UseSearch      bool
		AttachmentUrls []string
		RoleID         int
		ModelID        int
		Model          string
	}

	SaveMessageArgs struct {
		ReplyID       int
		ReplyContent  string
		Type          schema.RoleType
		ReasonContent string
		UseContext    bool
		Segs          []*types.KnowledgeSegment
		InputInfo     *InputInfo
	}

	InputInfo struct {
		history   []*ent.AiChatMessage
		Segs      []*types.KnowledgeSegment
		WebSearch *types.WebSearchResult
		conv      *ent.AiChatConversation
		SendArgs  *SendMessageArgs
		RoleID    int
		ModelID   int
	}

	ChatRecord struct {
		InnerRecord *types.ChatInnerRecord
		UserMsg     *ent.AiChatMessage
		AiMsg       *ent.AiChatMessage
		Output      *schema.Message
	}
)

type invokeFN func(ctx context.Context, graph compose.Runnable[map[string]any, *schema.Message], tmlv map[string]any,
	userMsg, aiMsg *ent.AiChatMessage, onChunkFN OnChunkFN, onFirstChunkFN OnFirstChunkFN, opts ...compose.Option) (*schema.Message, error)
type OnChunkFN func(chunk *schema.Message, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error
type OnFirstChunkFN func(chunk *schema.Message, userMsg, aiMsg *ent.AiChatMessage, record *types.ChatInnerRecord) error

func NewChatBiz(cc data.ChatConversationClient, rc data.RoleClient, kc data.KnowledgeClient, mc data.ModelClient,
	cmc data.ChatMessageClient, wc data.WebPageClient, tc data.ToolClient, loader url.Loader, mcm aimcp.MCPClientManager,
	wsm *Searcher, vs vector.VectorStore, conf *conf.Bootstrap, tr *factory.ToolRegistry, l log.Logger) ChatBiz {
	return &chatBiz{
		cc:        cc,
		rc:        rc,
		kc:        kc,
		mc:        mc,
		cmc:       cmc,
		wc:        wc,
		tc:        tc,
		urlReader: loader,
		mcm:       mcm,
		vs:        vs,
		wsm:       wsm,
		conf:      conf,
		tr:        tr,
		l:         log.NewHelper(l, log.WithMessageKey("biz-chat")),
	}
}

func (b *chatBiz) GetRoleInfo(ctx context.Context, roleID int) (*types.RoleInfo, error) {
	if roleID == 0 {
		return nil, nil
	}
	role, err := b.rc.GetActiveByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	roleInfo := &types.RoleInfo{
		KnowLedgeIDs:   role.KnowledgeIds,
		ToolIDs:        role.ToolIds,
		MCPClientNames: role.McpClientNames,
	}
	return roleInfo, nil
}

func (b *chatBiz) SaveMessage(ctx context.Context, args *data.CreateChatMessageArgs) (*ent.AiChatMessage, error) {
	cmc, tx, ctx, err := data.WithTx(ctx, b.cmc)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	msg, err := cmc.Create(ctx, args)
	if err != nil {
		err := data.Rollback(tx)
		if err != nil {
			return nil, fmt.Errorf("failed to create message: %w", err)
		}
	}

	if err := data.Commit(tx); err != nil {
		return nil, fmt.Errorf("failed to commit message creation: %w", err)
	}
	return msg, nil
}

func (b *chatBiz) PrepareInput(ctx context.Context, args *SendMessageArgs) (*InputInfo, error) {
	u := trans.FromContext(ctx)
	// 1.1 校验对话存在
	conversation, err := b.cc.GetByID(ctx, args.ConversationID)
	if err != nil || conversation == nil || conversation.UserID != int(u.Id) {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	msgList, err := b.cmc.List(ctx, &data.ListChatMessageArgs{
		PaginationArgs: &db.PaginationArgs{
			Page:     1,
			PageSize: 20,
			OrderBy:  aimodel.FieldUpdatedAt,
			OrderDir: db.OrderDirectionDesc,
		},
		ConversationID: conversation.ID,
	})
	if err != nil {
		return nil, commonpb.ErrorDb("failed to list messages: %w", err)
	}
	// 1.2 校验模型是否存在
	model, err := b.mc.GetActiveModelByIDType(ctx, conversation.ModelID, types.ModelTypeChat)
	if err != nil || model == nil {
		return nil, commonpb.ErrorParamInvalid("invalid model")
	}

	return &InputInfo{
		history:  msgList.ChatMessages,
		SendArgs: args,
		conv:     conversation,
		RoleID:   conversation.RoleID,
		ModelID:  conversation.ModelID,
	}, nil
}

func (b *chatBiz) PageConversationMessage(ctx context.Context, conversationID int, pagination *db.PaginationArgs) (*data.ListChatMessageResult, error) {
	u := trans.FromContext(ctx)
	// 构建分页参数
	args := &data.ListChatMessageArgs{
		PaginationArgs: pagination,
		ConversationID: conversationID,
		UserID:         int(u.Id),
	}
	messages, err := b.cmc.List(ctx, args)
	if err != nil {
		return nil, err
	}
	// 查询关联网页
	mids := lo.Map(messages.ChatMessages, func(m *ent.AiChatMessage, index int) int {
		return m.ID
	})
	webPages, err := b.wc.ListByMessageIDs(ctx, mids)
	if err != nil {
		return nil, fmt.Errorf("failed to list web pages: %w", err)
	}
	var pageMap map[int][]*types.WebPage
	for _, p := range webPages {
		pageMap[p.MessageID] = append(pageMap[p.MessageID], &types.WebPage{
			Name:      p.Name,
			Icon:      p.Icon,
			Title:     p.Title,
			URL:       p.URL,
			Snippet:   p.Snippet,
			Summary:   p.Summary,
			MessageID: p.MessageID,
		})
	}
	messages.PageMap = pageMap

	return messages, nil
}

func (b *chatBiz) DeleteMessage(ctx context.Context, id int) error {
	u := trans.FromContext(ctx)
	// 1. 校验消息是否存在
	existed, err := b.cmc.GetByID(ctx, id)
	if err != nil || existed == nil || existed.UserID != int(u.Id) {
		return fmt.Errorf("failed to get message: %w", err)
	}
	// 2. 删除消息
	return b.cmc.Delete(ctx, existed.ID)
}

func (b *chatBiz) DeleteMessagesByConversationID(ctx context.Context, conversationID int) error {
	u := trans.FromContext(ctx)
	// 1. 校验会话是否存在
	existed, err := b.cc.GetByID(ctx, conversationID)
	if err != nil || existed == nil || existed.UserID != int(u.Id) {
		return fmt.Errorf("failed to get message: %w", err)
	}
	// 2. 删除会话下的所有消息
	if _, err := b.cmc.DeleteByConversationID(ctx, existed.ID); err != nil {
		return err
	}
	return nil
}

func (b *chatBiz) ListMessagesByConversationID(ctx context.Context, conversationID int, pagination *db.PaginationArgs) (*data.ListChatMessageResult, error) {
	u := trans.FromContext(ctx)
	// 1. 校验会话是否存在
	existed, err := b.cc.GetByID(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	// 2. 查询会话下的所有消息
	args := &data.ListChatMessageArgs{
		PaginationArgs: pagination,
		ConversationID: existed.ID,
		UserID:         int(u.Id),
	}
	return b.cmc.List(ctx, args)
}

func (b *chatBiz) CreateConversation(ctx context.Context, roleID int, modelID int, knowledgeID int) (*ent.AiChatConversation, error) {
	u := trans.FromContext(ctx)
	var err error
	// 获取聊天模型 model
	var model *ent.AiModel
	if modelID == 0 {
		model, err = b.mc.GetDefaultModel(ctx, types.ModelTypeChat)
	} else {
		model, err = b.mc.GetActiveModelByIDType(ctx, modelID, types.ModelTypeChat)
	}
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("invalid model id %s", modelID)
	}
	// 校验知识库存在性
	if knowledgeID != 0 {
		_, err = b.kc.GetActiveByID(ctx, knowledgeID)
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("invalid knowledge id %s", knowledgeID)
		}
	}

	// 创建聊天会话
	conversation := &data.UpsertChatConversationParams{
		Pinned:      false,
		UserID:      int(u.Id),
		ModelID:     model.ID,
		Model:       model.Name,
		Temperature: model.Temperature,
		MaxTokens:   model.MaxTokens,
		MaxContexts: model.MaxContext,
	}
	if roleID != 0 {
		// 获取聊天角色 role
		role, err := b.rc.GetActiveByID(ctx, roleID)
		if err != nil || role == nil {
			return nil, fmt.Errorf("failed to get role: %w", err)
		}
		conversation.Title = role.Name
		conversation.RoleID = role.ID
		conversation.SysMsg = role.SystemMessage
	} else {
		conversation.Title = defaultTitle
	}
	return b.cc.Upsert(ctx, conversation)
}

func (b *chatBiz) UpdateConversation(ctx context.Context, args *UpdateConversationArgs) (*ent.AiChatConversation, error) {
	u := trans.FromContext(ctx)
	// 1.1 校验对话是否存在
	existed, err := b.cc.GetByID(ctx, args.ExistedID)
	if err != nil || existed == nil || existed.UserID != int(u.Id) {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	params := &data.UpsertChatConversationParams{
		Existed:     existed,
		Title:       args.Title,
		Pinned:      args.Pinned,
		SysMsg:      args.SysMsg,
		Temperature: args.Temperature,
		MaxTokens:   args.MaxTokens,
		MaxContexts: args.MaxContexts,
	}
	// 1.2 校验模型是否存在
	if args.ModelID != 0 {
		m, err := b.mc.GetActiveModelByIDType(ctx, args.ModelID, types.ModelTypeChat)
		if err != nil || m == nil {
			return nil, fmt.Errorf("failed to get model: %w", err)
		}
		params.ModelID = m.ID
		params.Model = m.Name
	}

	// 2. 更新对话信息
	return b.cc.Upsert(ctx, params)
}

func (b *chatBiz) ListConversationByUser(ctx context.Context) ([]*ent.AiChatConversation, error) {
	u := trans.FromContext(ctx)
	return b.cc.ListByUserID(ctx, int(u.Id))
}

func (b *chatBiz) GetConversation(ctx context.Context, id int) (*ent.AiChatConversation, error) {
	return b.cc.GetByID(ctx, id)
}

func (b *chatBiz) DeleteConversation(ctx context.Context, args *DeleteConversationArgs) error {
	u := trans.FromContext(ctx)
	if args.Unpinned {
		// 删除未置顶对话
		_, err := b.cc.DeleteUnpinnedByUserID(ctx, int(u.Id))
		return err
	}
	// 1. 校验对话是否存在
	existed, err := b.cc.GetByID(ctx, args.ID)
	if err != nil || existed == nil || existed.UserID != int(u.Id) {
		return fmt.Errorf("failed to get conversation: %w", err)
	}
	// 2. 删除对话
	return b.cc.Delete(ctx, existed.ID)
}

func (b *chatBiz) PageConversation(ctx context.Context, args *data.ListChatConversationArgs) (*data.ListChatConversationResult, error) {
	return b.cc.List(ctx, args)
}

func (b *chatBiz) BuildInput(info *InputInfo) []*schema.Message {
	var res []*schema.Message
	// 1.1 添加系统消息
	if info.conv.SystemMessage != "" {
		res = append(res, schema.SystemMessage(info.conv.SystemMessage))
	}
	// 1.2 历史消息
	msgs := b.filterContextMessages(info.history, info.conv, info.SendArgs)
	res = append(res, msgs...)

	// 1.3 当前消息
	res = append(res, schema.UserMessage(info.SendArgs.Content))

	// 1.4 知识库
	if len(info.Segs) > 0 {
		var refer strings.Builder
		for _, seg := range info.Segs {
			refer.WriteString("<Reference>")
			refer.WriteString(seg.Content)
			refer.WriteString("</Reference>\n\n")
		}
		res = append(res, schema.UserMessage(refer.String()))
	}

	// 1.6 附件
	if len(info.SendArgs.AttachmentUrls) > 0 {
		res = append(res, b.buildAttachmentUserMessage(info.SendArgs.AttachmentUrls))
	}
	return res
}

func (b *chatBiz) filterContextMessages(h []*ent.AiChatMessage, c *ent.AiChatConversation, params *SendMessageArgs) []*schema.Message {
	if c.MaxContexts == 0 || !params.UseContext {
		return nil
	}
	contextMsgs := make([]*schema.Message, 0, c.MaxContexts*2)
	for i := len(h) - 1; i > 0; i-- {
		aiMsg := h[i]
		userMsg := h[i-1]
		// 跳过无效消息对
		if !b.isValidMessagePair(aiMsg, userMsg) {
			continue
		}
		contextMsgs = append(contextMsgs, schema.AssistantMessage(aiMsg.Content, nil))
		contextMsgs = append(contextMsgs, schema.UserMessage(userMsg.Content))
		// 解析 附件URL，转成用户消息
		urls := strings.Split(strings.TrimSpace(aiMsg.Content), ",")
		contextMsgs = append(contextMsgs, b.buildAttachmentUserMessage(urls))
		// 超过最大上下文，结束
		if len(contextMsgs) >= c.MaxContexts*2 {
			break
		}
	}
	mutable.Reverse(contextMsgs)
	return contextMsgs
}

// isValidMessagePair 检查消息对是否有效
func (b *chatBiz) isValidMessagePair(aiMsg, userMsg *ent.AiChatMessage) bool {
	// 检查消息是否为 nil
	if aiMsg == nil || userMsg == nil {
		return false
	}

	// 检查消息内容是否为空
	if aiMsg.Content == "" {
		return false
	}

	// 检查消息是否属于同一回复链
	if aiMsg.ReplyID == 0 || userMsg.ReplyID != aiMsg.ReplyID {
		return false
	}

	return true
}

func (b *chatBiz) buildAttachmentUserMessage(attachmentUrls []string) *schema.Message {
	if len(attachmentUrls) == 0 {
		return nil
	}
	attachmentContents := make(map[string]string, len(attachmentUrls))
	for _, au := range attachmentUrls {
		name := filepath.Base(au)
		mimeType := mime.TypeByExtension(filepath.Ext(name))
		var (
			content string
			err     error
		)
		if isImage(mimeType) {
			content, err = downloadAsBase64(au)
			if err != nil {
				b.l.Errorf("failed to read attachment %s: %v", au, err)
				continue
			}
		} else {
			var docs []*schema.Document
			// 1. 下载并解析
			docs, err = b.urlReader.Load(context.Background(), document.Source{URI: au})
			if err != nil {
				b.l.Errorf("failed to read attachment %s: %v", au, err)
				continue
			}
			// 2. 取第一个文档
			if len(docs) == 0 || strings.TrimSpace(docs[0].Content) == "" {
				b.l.Errorf("url(%s) 文件内容为空", au)
			}

			// 3. 多页文档（如 PDF）合并所有页内容
			var sb strings.Builder
			for _, doc := range docs {
				sb.WriteString(doc.Content)
				sb.WriteString("\n")
			}
			content = strings.TrimSpace(sb.String())
		}
		if content != "" {
			attachmentContents[name] = content
		}
	}

	// 拼接 附件内容
	parts := make([]string, 0, len(attachmentContents))
	for k, v := range attachmentContents {
		parts = append(parts, fmt.Sprintf(`<Attachment name="%s">%s</Attachment>`, k, v))
	}
	attachment := strings.Join(parts, "\n\n")
	return schema.UserMessage(fmt.Sprintf(attachmentUserMessageTemplate, attachment))
}

func (b *chatBiz) GetToolInfosByRoleID(info *types.RoleInfo) ([]*schema.ToolInfo, error) {
	var toolCallbacks []*schema.ToolInfo
	// 1. 通过 toolIDs 查询 tool 工具
	if len(info.ToolIDs) != 0 {
		tools, err := b.tc.GetByIDs(context.Background(), info.ToolIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get tools: %w", err)
		}
		for _, t := range tools {
			tInfo := &types.ToolInfo{
				Name:       t.Name,
				Desc:       t.Description,
				Type:       t.Type,
				Parameters: t.Parameters,
			}
			toolCallbacks = append(toolCallbacks, message.ToToolInfo(tInfo))
		}
	}
	// 2. 通过 MCP 查询 tool 工具
	if len(info.MCPClientNames) != 0 {
		for _, clientName := range info.MCPClientNames {
			c, err := b.mcm.GetClient(clientName)
			if err != nil {
				b.l.Errorf("failed to get mcp client %s: %v", clientName, err)
				continue
			}
			ctx := context.Background()
			tools, err := mcp.GetTools(ctx, &mcp.Config{Cli: c})
			if err != nil {
				b.l.Errorf("failed to get mcp %s tools: %v", clientName, err)
				continue
			}
			for _, t := range tools {
				if tInfo, err := t.Info(ctx); err == nil {
					toolCallbacks = append(toolCallbacks, tInfo)
				}
			}
		}
	}
	return toolCallbacks, nil
}

func (b *chatBiz) PrepareTemplate(ctx context.Context, args *SendMessageArgs) (map[string]any, *ent.AiChatMessage, error) {
	u := trans.FromContext(ctx)
	// 1.1 校验对话存在
	conversation, err := b.cc.GetByID(ctx, args.ConversationID)
	if err != nil || conversation == nil || conversation.UserID != int(u.Id) {
		return nil, nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	// 1.2校验对话模型是否改变，若是，则更新对话
	if args.ModelID != conversation.ModelID {
		_, err := b.cc.UpdateModel(ctx, conversation.ID, args.ModelID, args.Model)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to update model: %w", err)
		}
	}
	args.RoleID = conversation.RoleID
	msgList, err := b.cmc.List(ctx, &data.ListChatMessageArgs{
		PaginationArgs: &db.PaginationArgs{
			Page:     1,
			PageSize: 20,
			OrderBy:  aimodel.FieldUpdatedAt,
			OrderDir: db.OrderDirectionDesc,
		},
		ConversationID: conversation.ID,
	})
	if err != nil {
		return nil, nil, commonpb.ErrorDb("failed to list messages: %w", err)
	}
	// 1.3 校验模型是否存在
	model, err := b.mc.GetActiveModelByIDType(ctx, conversation.ModelID, types.ModelTypeChat)
	if err != nil || model == nil {
		return nil, nil, commonpb.ErrorParamInvalid("invalid model")
	}
	// 2. 获取历史消息
	history := b.filterContextMessages(msgList.ChatMessages, conversation, args)

	// 3. 保存用户消息
	saveArgs := &data.CreateChatMessageArgs{
		CID:     conversation.ID,
		UserID:  int(u.Id),
		RoleID:  conversation.RoleID,
		ModelID: conversation.ModelID,
		Type:    string(schema.User),
		Content: args.Content,
	}
	if len(args.AttachmentUrls) > 0 {
		saveArgs.AttachUrls = args.AttachmentUrls
	}

	userMsg, err := b.SaveMessage(ctx, saveArgs)
	if err != nil {
		return nil, nil, commonpb.ErrorDb("failed to save message: %w", err)
	}
	return map[string]any{
		"system_message": conversation.SystemMessage,
		"query":          args.Content,
		"history":        history,
		"attachment":     b.buildAttachmentUserMessage(args.AttachmentUrls),
	}, userMsg, nil
}

func (b *chatBiz) invokeChat(ctx context.Context, args *SendMessageArgs, m emodel.ToolCallingChatModel, retrieveTool tool.InvokableTool,
	fn invokeFN, genLocalStateFN compose.GenLocalState[*types.ChatState], chunkFN OnChunkFN, firstFN OnFirstChunkFN) (*ChatRecord, error) {
	// 2.1 准备输入所需信息
	tmlv, userMsg, err := b.PrepareTemplate(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare template: %w", err)
	}

	// 2.2 获取角色信息
	roleInfo, err := b.GetRoleInfo(ctx, args.RoleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get role info: %w", err)
	}

	// 2.3 获取tools
	useRag := len(roleInfo.KnowLedgeIDs) == 0
	tools, err := b.BuildTools(retrieveTool, roleInfo, args.UseSearch, useRag)
	if err != nil {
		return nil, fmt.Errorf("failed to build tools: %w", err)
	}
	// 2.4 保存助手消息
	assistMsgArgs := &data.CreateChatMessageArgs{
		CID:     userMsg.ConversationID,
		UserID:  userMsg.UserID,
		RoleID:  userMsg.RoleID,
		ModelID: userMsg.ModelID,
		Type:    string(schema.Assistant),
		ReplyID: userMsg.ID,
	}
	aiMsg, err := b.SaveMessage(ctx, assistMsgArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to save message: %w", err)
	}

	g, err := b.buildChatGraph(m, tools, aiMsg.ID, compose.WithGenLocalState(genLocalStateFN))
	if err != nil {
		return nil, fmt.Errorf("failed to build chat graph: %w", err)
	}
	//output, err := g.Invoke(ctx, tmlv)
	output, err := fn(ctx, g, tmlv, userMsg, aiMsg, chunkFN, firstFN)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke chat graph: %w", err)
	}

	// 2.5 更新llm回复
	aiMsg, err = b.cmc.UpdateContent(ctx, aiMsg.ID, output.Content, output.ReasoningContent)
	if err != nil {
		return nil, fmt.Errorf("failed to update ai message content: %w", err)
	}
	return &ChatRecord{
		UserMsg: userMsg,
		AiMsg:   aiMsg,
		Output:  output,
	}, nil
}

func (b *chatBiz) Generate(ctx context.Context, args *SendMessageArgs, m emodel.ToolCallingChatModel,
	retrieveTool tool.InvokableTool) (*ChatRecord, error) {
	fn := func(ctx context.Context, graph compose.Runnable[map[string]any, *schema.Message], tmlv map[string]any,
		userMsg, aiMsg *ent.AiChatMessage, onChunkFN OnChunkFN, onFirstChunkFN OnFirstChunkFN,
		opts ...compose.Option) (*schema.Message, error) {
		return graph.Invoke(ctx, tmlv, opts...)
	}
	state := &types.ChatState{
		Record: &types.ChatInnerRecord{
			Segs:           make([]*types.KnowledgeSegment, 0),
			RouterDecision: make([]*schema.Message, 0),
			WebSearch: &types.WebSearchResult{
				WebPages: make([]*types.WebPage, 0),
			},
		},
	}
	genLocalStateFN := func(ctx context.Context) (state *types.ChatState) {
		return state
	}
	record, err := b.invokeChat(ctx, args, m, retrieveTool, fn, genLocalStateFN, nil, nil)
	if err != nil {
		return nil, err
	}
	record.InnerRecord = state.Record
	return record, nil
}

func (b *chatBiz) Stream(ctx context.Context, args *SendMessageArgs, m emodel.ToolCallingChatModel, retrieveTool tool.InvokableTool,
	onChunk OnChunkFN, onFirst OnFirstChunkFN) (*ChatRecord, error) {
	state := &types.ChatState{
		Record: &types.ChatInnerRecord{
			Segs:           make([]*types.KnowledgeSegment, 0),
			RouterDecision: make([]*schema.Message, 0),
			WebSearch: &types.WebSearchResult{
				WebPages: make([]*types.WebPage, 0),
			},
		},
	}
	genLocalStateFN := func(ctx context.Context) (state *types.ChatState) {
		return state
	}

	fn := func(ctx context.Context, graph compose.Runnable[map[string]any, *schema.Message], tmlv map[string]any,
		userMsg, aiMsg *ent.AiChatMessage, onChunkFN OnChunkFN, onFirstChunkFN OnFirstChunkFN,
		opts ...compose.Option) (*schema.Message, error) {
		streamResp, err := graph.Stream(ctx, tmlv, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to stream message: %w", err)
		}
		defer streamResp.Close()
		var (
			contentBuffer          strings.Builder
			reasoningContentBuffer strings.Builder
			isFirst                = true
			clientDisconnected     = false
		)

		for {
			chunk, err := streamResp.Recv()
			if err != nil {
				if err == io.EOF {
					break // 正常读取结束
				}

				// 优雅捕获 Context 取消的动作
				if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
					b.l.Info("request cancelled by user/context")
					return nil, ctx.Err()
				}

				return nil, fmt.Errorf("failed to read message: %w", err)
			}
			if !clientDisconnected {
				var err error
				if isFirst && onFirstChunkFN != nil {
					err = onFirstChunkFN(chunk, userMsg, aiMsg, state.Record)
					isFirst = false
				} else {
					err = onChunkFN(chunk, aiMsg, state.Record)
				}
				// 清空记录，重新记录
				state.Record.WebSearch.WebPages = make([]*types.WebPage, 0)
				state.Record.Segs = make([]*types.KnowledgeSegment, 0)
				state.Record.RouterDecision = make([]*schema.Message, 0)
				if err != nil {
					b.l.Warnf("failed to send message: %v", err)
					if contentBuffer.Len() == 0 {
						// 退出函数会触发 defer streamResp.Close()，彻底掐断与大模型的连接！
						return nil, fmt.Errorf("client aborted early: %w", err)
					}
					clientDisconnected = true
				}
			}

			contentBuffer.WriteString(chunk.Content)
			reasoningContentBuffer.WriteString(chunk.ReasoningContent)
		}
		return &schema.Message{
			Role:             schema.Assistant,
			Content:          contentBuffer.String(),
			ReasoningContent: reasoningContentBuffer.String(),
		}, nil
	}
	record, err := b.invokeChat(ctx, args, m, retrieveTool, fn, genLocalStateFN, onChunk, onFirst)
	if err != nil {
		return nil, err
	}
	record.InnerRecord = state.Record
	return record, nil
}

func (b *chatBiz) buildChatGraph(m emodel.ToolCallingChatModel, tools []tool.BaseTool, aiMsgID int, opts ...compose.NewGraphOption) (compose.Runnable[map[string]any, *schema.Message], error) {
	// 1. 构建输入模板
	tml := prompt.FromMessages(schema.FString,
		schema.SystemMessage("{system_message}"),
		schema.MessagesPlaceholder("history", true),
		schema.UserMessage("{query}"),
		schema.UserMessage("{attachment}"))

	// 2. 定义两个 tool：联网搜索 & 知识库检索
	ctx := context.Background()
	toolInfos := lo.FilterMap(tools, func(t tool.BaseTool, index int) (*schema.ToolInfo, bool) {
		info, err := t.Info(ctx)
		if err != nil {
			b.l.Warnf("failed to get tool info for tool %s: %v", t, err)
			return nil, false
		}
		return info, true
	})
	routerLLMWithTools, _ := m.WithTools(toolInfos)

	toolNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: tools,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tool node: %w", err)
	}

	g := compose.NewGraph[map[string]any, *schema.Message](opts...)
	g.AddChatTemplateNode("template", tml,
		compose.WithStatePreHandler(func(ctx context.Context, in map[string]any, state *types.ChatState) (map[string]any, error) {
			// 注入ai消息的id
			state.MsgID = aiMsgID
			return in, nil
		}),
		compose.WithStatePostHandler(func(ctx context.Context, out []*schema.Message, state *types.ChatState) ([]*schema.Message, error) {
			state.Messages = append(state.Messages, out...)
			return out, nil
		}))
	g.AddChatModelNode("router_llm", routerLLMWithTools,
		compose.WithStatePostHandler(func(ctx context.Context, out *schema.Message, state *types.ChatState) (*schema.Message, error) {
			state.Messages = append(state.Messages, out)
			state.Record.RouterDecision = append(state.Record.RouterDecision, out)
			return out, nil
		}))
	g.AddToolsNode("tools", toolNode,
		compose.WithStatePostHandler(func(ctx context.Context, out []*schema.Message, state *types.ChatState) ([]*schema.Message, error) {
			state.Messages = append(state.Messages, out...)
			return state.Messages, nil
		}))
	g.AddChatModelNode("answer_llm", m)
	g.AddEdge(compose.START, "template")
	g.AddEdge("template", "router_llm")
	g.AddBranch("router_llm", compose.NewGraphBranch(
		func(ctx context.Context, msg *schema.Message) (string, error) {
			if len(msg.ToolCalls) > 0 {
				return "tools", nil
			}
			return "answer_llm", nil // 无检索直接到 answer_llm，State.Messages 已经够用
		},
		map[string]bool{
			"tools":      true,
			"answer_llm": true,
		},
	))
	g.AddEdge("tools", "answer_llm")
	g.AddEdge("answer_llm", compose.END)
	return g.Compile(ctx)
}

func (b *chatBiz) BuildTools(retrieve tool.InvokableTool, info *types.RoleInfo, useSearch bool, useRag bool) ([]tool.BaseTool, error) {
	var bts []tool.BaseTool
	if useSearch {
		// 联网搜索
		search, err := NewBochaTool(b.conf.Extensions.Bocha, b.wc)
		if err != nil {
			return nil, fmt.Errorf("failed to create bocha tool: %w", err)
		}
		bts = append(bts, search)
	}

	if useRag {
		// 知识库检索
		bts = append(bts, retrieve)
	}
	// 1. 通过 toolIDs 查询 tool 工具
	if len(info.ToolIDs) != 0 {
		tools, err := b.tc.GetByIDs(context.Background(), info.ToolIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get tools: %w", err)
		}
		for _, t := range tools {
			tInfo := &factory.ToolConfig{
				Name: t.Name,
				Desc: t.Description,
				Type: t.Type,
				//Parameters: t.Parameters,
			}
			it, err := b.tr.BuildTool(tInfo)
			if err != nil {
				return nil, fmt.Errorf("failed to build tool %s: %w", t.Name, err)
			}
			bts = append(bts, it)
		}
	}
	// 2. 通过 MCP 查询 tool 工具
	if len(info.MCPClientNames) != 0 {
		for _, clientName := range info.MCPClientNames {
			c, err := b.mcm.GetClient(clientName)
			if err != nil {
				b.l.Errorf("failed to get mcp client %s: %v", clientName, err)
				continue
			}
			ctx := context.Background()
			tools, err := mcp.GetTools(ctx, &mcp.Config{Cli: c})
			if err != nil {
				b.l.Errorf("failed to get mcp %s tools: %v", clientName, err)
				continue
			}
			bts = append(bts, tools...)
		}
	}

	return bts, nil
}

func downloadAsBase64(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http status code %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func isImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}
