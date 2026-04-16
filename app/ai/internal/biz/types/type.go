package types

import (
	"entmodule"
	"sync"

	"github.com/cloudwego/eino/schema"
)

type (
	RoleInfo struct {
		KnowLedgeIDs   []int
		ToolIDs        []int
		MCPClientNames []string
	}

	KnowledgeSegment struct {
		ID          int              `json:"id"`
		DocumentID  int              `json:"document_id"`
		KnowledgeID int              `json:"knowledge_id"`
		Content     string           `json:"content"`
		ContentLen  int              `json:"content_len"`
		Tokens      int              `json:"tokens"`
		Score       float64          `json:"score"`
		VectorID    string           `json:"vector_id"`
		Status      entmodule.Status `json:"status"`
	}
)

type Strategy string

const (
	StrategyAuto           Strategy = "auto"
	StrategyMarkdownHeader Strategy = "markdown_header"
	StrategySemantic       Strategy = "semantic"
	StrategyParagraph      Strategy = "paragraph"
	StrategySentence       Strategy = "sentence"
)

const (
	MilvusIDField           = "id"
	MilvusDocumentIDField   = "doc_id"
	MilvusKnowledgeIDField  = "kb_id"
	MilvusContentField      = "content"
	MilvusVectorFieldField  = "vector"
	MilvusSparseVectorField = "sparse_vector"
	MilvusMetadataField     = "metadata"
)

type ImageStatus string

// Ai Image Status
const (
	ImageStatusProcessing ImageStatus = "processing"
	ImageStatusSuccess    ImageStatus = "success"
	ImageStatusFailed     ImageStatus = "failed"
	ImageStatusUnknown    ImageStatus = "unknown"
)

func (ImageStatus) Values() []string {
	return []string{
		string(ImageStatusProcessing),
		string(ImageStatusSuccess),
		string(ImageStatusFailed),
	}
}

var ImageStatusProtoValues = map[string]int32{
	string(ImageStatusProcessing): 1,
	string(ImageStatusSuccess):    2,
	string(ImageStatusFailed):     3,
}

var ProtoImageStatusValues = map[int32]ImageStatus{
	1: ImageStatusProcessing,
	2: ImageStatusSuccess,
	3: ImageStatusFailed,
}

type ModelType string

// Ai Model Type
const (
	ModelTypeChat  string = "chat"
	ModelTypeImage string = "image"
	ModelTypeVideo string = "video"
	ModelTypeAudio string = "audio"
)

func (ModelType) Values() []string {
	return []string{
		ModelTypeChat,
		ModelTypeImage,
		ModelTypeVideo,
		ModelTypeAudio,
	}
}

var ModelTypeProtoValues = map[string]int32{
	ModelTypeChat:  1,
	ModelTypeImage: 2,
	ModelTypeVideo: 3,
	ModelTypeAudio: 4,
}

// DocumentStatus Ai Document Status
type DocumentStatus string

const (
	DocumentPending    DocumentStatus = "pending"
	DocumentProcessing DocumentStatus = "processing"
	DocumentSuccess    DocumentStatus = "success"
	DocumentFailed     DocumentStatus = "failed"
)

func (DocumentStatus) Values() []string {
	return []string{
		string(DocumentProcessing),
		string(DocumentSuccess),
		string(DocumentFailed),
		string(DocumentPending),
	}
}

// VectorDim Vector Dimension
const VectorDim = 1024

type SearchClient string

const (
	SerperSearchClient SearchClient = "serper"
	BraveSearchClient  SearchClient = "brave"
	BochaSearchClient  SearchClient = "bocha"
)

// SearchResult Search Result of Vector Store
type SearchResult struct {
	ID         string
	DocumentID string
	Content    string
	Score      float32
}

// chat
type (
	WebSearchResult struct {
		Total    int
		WebPages []*WebPage
	}
)

type (
	ToolInfo struct {
		ID         int    `json:"id"`
		Name       string `json:"name"`       // 对应 schema.ToolInfo.Name
		Desc       string `json:"desc"`       // 对应 schema.ToolInfo.Desc
		Type       string `json:"type"`       // "http" | "builtin" | "mcp" 等
		Parameters string `json:"parameters"` // JSON Schema 字符串，存 object 类型的完整 schema
		Endpoint   string `json:"endpoint"`   // HTTP tool 需要，内置 tool 留空
		Method     string `json:"method"`     // HTTP tool 需要，如 "POST"
	}
)

type WebPage struct {
	Name      string
	Icon      string
	Title     string
	URL       string
	Snippet   string
	Summary   string
	MessageID int
}

type (
	ChatState struct {
		mu       sync.Mutex
		MsgID    int
		Record   *ChatInnerRecord
		Messages []*schema.Message
	}

	ChatInnerRecord struct {
		WebSearch      *WebSearchResult
		Segs           []*KnowledgeSegment
		RouterDecision []*schema.Message
	}
)

type TextParseType string

const (
	TextMarkdown TextParseType = "markdown"
	TextPDF      TextParseType = "pdf"
	TextHTML     TextParseType = "html"
	TextJSON     TextParseType = "json"
	TextWord     TextParseType = "word"
	TextExcel    TextParseType = "excel"
	TextCSV      TextParseType = "csv"
	TextTxt      TextParseType = "txt"
	TextUnknown  TextParseType = "unknown"
)
const MaxFileSize = 1024 * 1024 * 10 // 10MB max

type TextParseSupport struct {
	Types       []TextParseType
	MaxFileSize int64
}
type SegmentSearchArgs struct {
	KnowledgeID  int
	Content      string  `json:"content"       jsonschema:"description=The query text used for semantic retrieval,required"`
	TopK         int     `json:"top_k"         jsonschema:"description=Number of most similar segments to return,minimum=1,maximum=100,required"`
	Similarity   float64 `json:"similarity"    jsonschema:"description=Similarity threshold between 0 and 1; results below this value will be filtered out,minimum=0,maximum=1"`
	KnowledgeIDs []int   `json:"knowledge_ids" jsonschema:"description=List of knowledge base IDs to search; searches all if empty"`
}
