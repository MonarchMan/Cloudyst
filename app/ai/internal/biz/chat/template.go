package chat

const (
	knowledgeUserMessageTemplate = "使用 <Reference></Reference> 标记中的内容作为本次对话的参考:\n\n" +
		"%s\n\n" + // 多个 <Reference></Reference> 的拼接
		"回答要求：\n- 避免提及你是从 <Reference></Reference> 获取的知识。"

	attachmentUserMessageTemplate = "使用 <Attachment></Attachment> 标记用户对话上传的附件内容:\n\n" +
		"%s\n\n" + // 多个 <Attachment></Attachment> 的拼接
		"回答要求：\n- 避免提及 <Attachment></Attachment> 附件的编码格式。"

	webSearchUserMessageTemplate = "使用 <WebSearch></WebSearch> 标记中的内容作为本次对话的参考:\n\n" +
		"%s\n\n" + // 多个 <WebSearch></WebSearch> 的拼接
		"回答要求：\n- 避免提及你是从 <WebSearch></WebSearch> 获取的知识。"
)
