package vector

import (
	"ai/internal/biz/types"
	"ai/internal/conf"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/samber/lo"
)

type VectorStore interface {
	// Upsert 写入或覆盖单个向量切片。
	Upsert(ctx context.Context, chunk *VectorChunk) (string, error)

	// BatchUpsert 批量写入或覆盖向量切片。
	BatchUpsert(ctx context.Context, chunks []*VectorChunk) ([]string, error)

	// DeleteByDocID 按业务文档 ID 删除所有切片 (极度常用：用户在界面上点击"删除该文件"时触发)
	DeleteByDocID(ctx context.Context, kbID int, docID int) error

	// DeleteByDocIDs 按业务文档 IDs 删除所有切片
	DeleteByDocIDs(ctx context.Context, docIDs []int) error

	// CountChunksByDocID 获取某文档切片数量/详情 (用于后台管理或对账)
	DeleteByDocIDsInKB(ctx context.Context, kbID int, docIDs []int) error

	DeleteByKBID(ctx context.Context, kbID int) error

	DeleteByKBIDs(ctx context.Context, kbIDs []int) error

	CountChunksByDocID(ctx context.Context, kbID int, docID int) (int64, error)

	Search(ctx context.Context, req *SearchRequest) ([]*SearchHit, error)

	// SearchKnowledge 搜索知识库，返回 topK 个最相关的切片 ID
	SearchKnowledge(ctx context.Context, kbID string, queryVectors [][]float32, topK int) ([]string, error)

	// Delete 删除切片
	Delete(ctx context.Context, ids []string) error

	// GetContentByIDs 获取切片内容
	GetContentByIDs(ctx context.Context, ids []string) ([]string, error)

	// EnsureCollection 确保集合存在
	EnsureCollection(ctx context.Context) error
}

type SearchMode string

const (
	SearchModeDense  SearchMode = "dense"
	SearchModeSparse SearchMode = "sparse"
)

type VectorChunk struct {
	ID          string
	KnowledgeID int64
	DocumentID  int64
	Content     string
	Vector      []float32
	Metadata    map[string]any
}

type SearchRequest struct {
	Mode         SearchMode
	TopK         int
	QueryVector  []float32
	QueryVectors [][]float32
	QueryText    string
	VectorField  string
	Filter       string
	KnowledgeIDs []int
	DocumentIDs  []int
	OutputFields []string
	SearchParams map[string]string
}

type SearchHit struct {
	ID          string
	KnowledgeID int64
	DocumentID  int64
	Content     string
	Score       float32
	Metadata    map[string]any
}

func NewMilvusClient(bs *conf.Bootstrap) (VectorStore, func(), error) {
	cfg := bs.Data.Milvus
	client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{
		Address:  cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create milvus client: %w", err)
	}
	cleanup := func() {
		client.Close(context.Background())
	}
	denseMetric := entity.COSINE
	sparseMetric := entity.BM25
	if cfg.MetricType != nil {
		if cfg.MetricType.Dense != "" {
			denseMetric = entity.MetricType(cfg.MetricType.Dense)
		}
		if cfg.MetricType.Sparse != "" {
			sparseMetric = entity.MetricType(cfg.MetricType.Sparse)
		}
	}
	return &milvusClient{
		client:       client,
		collection:   cfg.Collection,
		denseMetric:  denseMetric,
		sparseMetric: sparseMetric,
	}, cleanup, nil
}

type milvusClient struct {
	client       *milvusclient.Client
	collection   string
	denseMetric  entity.MetricType
	sparseMetric entity.MetricType
}

func (c *milvusClient) GetByIDs(ctx context.Context, ids []string) (milvusclient.ResultSet, error) {
	option := milvusclient.NewQueryOption(c.collection).
		WithIDs(column.NewColumnString(types.MilvusIDField, ids))
	return c.client.Get(ctx, option)
}

func (c *milvusClient) GetContentByIDs(ctx context.Context, ids []string) ([]string, error) {
	option := milvusclient.NewQueryOption(c.collection).
		WithIDs(column.NewColumnString(types.MilvusIDField, ids)).
		WithOutputFields(types.MilvusIDField, types.MilvusContentField)
	rs, err := c.client.Get(ctx, option)
	if err != nil {
		return nil, fmt.Errorf("failed to get the content by ids: %w", err)
	}
	// 建立 id -> content 映射，保持入参顺序返回
	contentMap := make(map[string]string, rs.ResultCount)
	for i := range rs.ResultCount {
		id, _ := rs.GetColumn(types.MilvusIDField).GetAsString(i)
		content, _ := rs.GetColumn(types.MilvusContentField).GetAsString(i)
		contentMap[id] = content
	}

	contents := make([]string, len(ids))
	for i, id := range ids {
		contents[i] = contentMap[id] // 未找到时保持空字符串
	}
	return contents, nil
}

func (c *milvusClient) Upsert(ctx context.Context, chunk *VectorChunk) (string, error) {
	ids, err := c.BatchUpsert(ctx, []*VectorChunk{chunk})
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("upsert returned no ids")
	}
	return ids[0], nil
}

func (c *milvusClient) BatchUpsert(ctx context.Context, chunks []*VectorChunk) ([]string, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	columns, err := vectorChunksToColumns(chunks)
	if err != nil {
		return nil, err
	}
	option := milvusclient.NewColumnBasedInsertOption(c.collection, columns...)
	result, err := c.client.Upsert(ctx, option)
	if err != nil {
		return nil, fmt.Errorf("upsert vector chunks: %w", err)
	}

	ids := make([]string, 0, len(chunks))
	if result.IDs != nil {
		for i := 0; i < result.IDs.Len(); i++ {
			id, err := result.IDs.GetAsString(i)
			if err != nil {
				return nil, fmt.Errorf("parse upsert id: %w", err)
			}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		for _, chunk := range chunks {
			ids = append(ids, chunk.ID)
		}
	}
	return ids, nil
}

func vectorChunksToColumns(chunks []*VectorChunk) ([]column.Column, error) {
	ids := make([]string, 0, len(chunks))
	kbIDs := make([]int64, 0, len(chunks))
	docIDs := make([]int64, 0, len(chunks))
	contents := make([]string, 0, len(chunks))
	vectors := make([][]float32, 0, len(chunks))
	metadatas := make([][]byte, 0, len(chunks))

	for i, chunk := range chunks {
		if chunk == nil {
			return nil, fmt.Errorf("vector chunk %d is nil", i)
		}
		if chunk.ID == "" {
			return nil, fmt.Errorf("vector chunk %d id is empty", i)
		}
		if len(chunk.Vector) == 0 {
			return nil, fmt.Errorf("vector chunk %s vector is empty", chunk.ID)
		}

		metadata := chunk.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata for chunk %s: %w", chunk.ID, err)
		}

		ids = append(ids, chunk.ID)
		kbIDs = append(kbIDs, chunk.KnowledgeID)
		docIDs = append(docIDs, chunk.DocumentID)
		contents = append(contents, chunk.Content)
		vectors = append(vectors, chunk.Vector)
		metadatas = append(metadatas, metadataBytes)
	}

	return []column.Column{
		column.NewColumnVarChar(types.MilvusIDField, ids),
		column.NewColumnInt64(types.MilvusKnowledgeIDField, kbIDs),
		column.NewColumnInt64(types.MilvusDocumentIDField, docIDs),
		column.NewColumnVarChar(types.MilvusContentField, contents),
		column.NewColumnFloatVector(types.MilvusVectorFieldField, len(vectors[0]), vectors),
		column.NewColumnJSONBytes(types.MilvusMetadataField, metadatas),
	}, nil
}

func (c *milvusClient) DeleteByDocID(ctx context.Context, kbID int, docID int) error {
	expr := fmt.Sprintf("%s == %d AND %s == %d", types.MilvusKnowledgeIDField, kbID, types.MilvusDocumentIDField, docID)
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).
		WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for doc %d: %w", docID, err)
	}
	return nil
}

func (c *milvusClient) DeleteByDocIDs(ctx context.Context, docIDs []int) error {
	if len(docIDs) == 0 {
		return nil
	}
	expr := intInFilter(types.MilvusDocumentIDField, docIDs)
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for docs: %w", err)
	}
	return nil
}

func (c *milvusClient) DeleteByDocIDsInKB(ctx context.Context, kbID int, docIDs []int) error {
	if len(docIDs) == 0 {
		return nil
	}
	expr := fmt.Sprintf("%s == %d AND %s", types.MilvusKnowledgeIDField, kbID, intInFilter(types.MilvusDocumentIDField, docIDs))
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for kb %d docs: %w", kbID, err)
	}
	return nil
}

func (c *milvusClient) DeleteByKBID(ctx context.Context, kbID int) error {
	expr := fmt.Sprintf("%s == %d", types.MilvusKnowledgeIDField, kbID)
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for kb %d: %w", kbID, err)
	}
	return nil
}

func (c *milvusClient) DeleteByKBIDs(ctx context.Context, kbIDs []int) error {
	if len(kbIDs) == 0 {
		return nil
	}
	expr := intInFilter(types.MilvusKnowledgeIDField, kbIDs)
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for kbs: %w", err)
	}
	return nil
}

func (c *milvusClient) CountChunksByDocID(ctx context.Context, kbID int, docID int) (int64, error) {
	expr := fmt.Sprintf("%s == %d AND %s == %d", types.MilvusKnowledgeIDField, kbID, types.MilvusDocumentIDField, docID)
	// 查询统计信息，只返回 count(*)
	// 现代版本的 Milvus Go SDK 支持使用 aggregation 快速 Count
	option := milvusclient.NewQueryOption(c.collection).
		WithFilter(expr).
		WithOutputFields("count(*)")

	res, err := c.client.Query(ctx, option)
	if err != nil {
		return 0, fmt.Errorf("query chunks failed: %w", err)
	}

	// 直接按列名获取，然后调用内置转换方法
	// 这里的列名固定为 "count(*)"
	count, err := res.GetColumn("count(*)").GetAsInt64(0)
	if err != nil {
		return 0, fmt.Errorf("failed to parse count value: %w", err)
	}

	return count, nil
}

func (c *milvusClient) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	option := milvusclient.NewDeleteOption(c.collection).
		WithStringIDs(types.MilvusIDField, ids)

	_, err := c.client.Delete(ctx, option)
	return err
}

func (c *milvusClient) Search(ctx context.Context, req *SearchRequest) ([]*SearchHit, error) {
	if req == nil {
		return nil, fmt.Errorf("search request is nil")
	}
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	fieldName := req.VectorField
	mode := req.Mode
	if mode == "" {
		mode = SearchModeDense
	}

	var vectors []entity.Vector
	switch mode {
	case SearchModeSparse:
		if req.QueryText == "" {
			return nil, fmt.Errorf("sparse search query text is empty")
		}
		if fieldName == "" {
			fieldName = types.MilvusSparseVectorField
		}
		vectors = []entity.Vector{entity.Text(req.QueryText)}
	case SearchModeDense:
		if fieldName == "" {
			fieldName = types.MilvusVectorFieldField
		}
		queryVectors := req.QueryVectors
		if len(queryVectors) == 0 && len(req.QueryVector) > 0 {
			queryVectors = [][]float32{req.QueryVector}
		}
		if len(queryVectors) == 0 {
			return nil, fmt.Errorf("dense search query vector is empty")
		}
		vectors = lo.Map(queryVectors, func(vec []float32, _ int) entity.Vector {
			return entity.FloatVector(vec)
		})
	default:
		return nil, fmt.Errorf("unsupported search mode: %s", mode)
	}

	outputFields := req.OutputFields
	if len(outputFields) == 0 {
		outputFields = []string{
			types.MilvusIDField,
			types.MilvusKnowledgeIDField,
			types.MilvusDocumentIDField,
			types.MilvusContentField,
			types.MilvusMetadataField,
		}
	}

	searchOpt := milvusclient.NewSearchOption(c.collection, topK, vectors).
		WithANNSField(fieldName).
		WithOutputFields(outputFields...)
	if filter := req.filterExpr(); filter != "" {
		searchOpt = searchOpt.WithFilter(filter)
	}
	if _, ok := req.SearchParams["metric_type"]; !ok {
		if mode == SearchModeSparse {
			searchOpt = searchOpt.WithSearchParam("metric_type", string(c.sparseMetric))
		} else {
			searchOpt = searchOpt.WithSearchParam("metric_type", string(c.denseMetric))
		}
	}
	for key, value := range req.SearchParams {
		searchOpt = searchOpt.WithSearchParam(key, value)
	}

	resultSets, err := c.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, fmt.Errorf("search vector chunks: %w", err)
	}
	return resultSetsToHits(resultSets), nil
}

func (r *SearchRequest) filterExpr() string {
	filters := make([]string, 0, 3)
	if r.Filter != "" {
		filters = append(filters, r.Filter)
	}
	if len(r.KnowledgeIDs) > 0 {
		filters = append(filters, intInFilter(types.MilvusKnowledgeIDField, r.KnowledgeIDs))
	}
	if len(r.DocumentIDs) > 0 {
		filters = append(filters, intInFilter(types.MilvusDocumentIDField, r.DocumentIDs))
	}
	return strings.Join(filters, " AND ")
}

func resultSetsToHits(resultSets []milvusclient.ResultSet) []*SearchHit {
	hits := make([]*SearchHit, 0)
	for _, resultSet := range resultSets {
		for i := 0; i < resultSet.ResultCount; i++ {
			hits = append(hits, &SearchHit{
				ID:          getStringColumn(resultSet, types.MilvusIDField, i),
				KnowledgeID: getInt64Column(resultSet, types.MilvusKnowledgeIDField, i),
				DocumentID:  getInt64Column(resultSet, types.MilvusDocumentIDField, i),
				Content:     getStringColumn(resultSet, types.MilvusContentField, i),
				Score:       getScore(resultSet, i),
				Metadata:    getJSONColumn(resultSet, types.MilvusMetadataField, i),
			})
		}
	}
	return hits
}

func getStringColumn(resultSet milvusclient.ResultSet, fieldName string, idx int) string {
	col := resultSet.GetColumn(fieldName)
	if col == nil {
		return ""
	}
	value, _ := col.GetAsString(idx)
	return value
}

func getInt64Column(resultSet milvusclient.ResultSet, fieldName string, idx int) int64 {
	col := resultSet.GetColumn(fieldName)
	if col == nil {
		return 0
	}
	value, _ := col.GetAsInt64(idx)
	return value
}

func getJSONColumn(resultSet milvusclient.ResultSet, fieldName string, idx int) map[string]any {
	col := resultSet.GetColumn(fieldName)
	if col == nil {
		return nil
	}
	raw, err := col.Get(idx)
	if err != nil {
		return nil
	}

	var bs []byte
	switch value := raw.(type) {
	case []byte:
		bs = value
	case string:
		bs = []byte(value)
	default:
		return nil
	}
	if len(bs) == 0 {
		return nil
	}

	metadata := make(map[string]any)
	if err := json.Unmarshal(bs, &metadata); err != nil {
		return nil
	}
	return metadata
}

func getScore(resultSet milvusclient.ResultSet, idx int) float32 {
	if idx < 0 || idx >= len(resultSet.Scores) {
		return 0
	}
	return resultSet.Scores[idx]
}

func (c *milvusClient) SearchKnowledge(ctx context.Context, kbID string, queryVectors [][]float32, topK int) ([]string, error) {

	// 1. 设置标量过滤条件：强制将搜索范围锁定在这个知识库内 (Partition Key 裁剪)
	expr := fmt.Sprintf("kb_id == '%s'", kbID)

	vectors := lo.Map(queryVectors, func(vec []float32, index int) entity.Vector {
		return entity.FloatVector(vec)
	})
	// 2. 🎯 V2 新写法：构造 SearchOption
	// 第一个参数是表名，第二个是 TopK 取多少条，第三个是你的查询向量
	searchOpt := milvusclient.NewSearchOption(c.collection, topK, vectors).
		WithANNSField(types.MilvusVectorFieldField).
		WithFilter(expr).                                               // 标量过滤
		WithOutputFields(types.MilvusIDField, types.MilvusContentField) // 指定要一同返回的标量列

	// 3. 执行搜索
	res, err := c.client.Search(ctx, searchOpt)
	if err != nil {
		return nil, err
	}

	// 4. 解析结果
	var contents []string
	for _, resultSet := range res {
		for i := 0; i < resultSet.ResultCount; i++ {
			// 获取我们要求返回的 content 字段
			contentStr, _ := resultSet.GetColumn(types.MilvusContentField).GetAsString(i)
			contents = append(contents, contentStr)
			// 这里也可以顺手拿到我们之前聊过的 _score：
			// score := resultSet.Scores[i]
		}
	}

	return contents, nil
}

func (c *milvusClient) EnsureCollection(ctx context.Context) error {
	has, err := c.client.HasCollection(ctx, milvusclient.NewHasCollectionOption(c.collection))
	if err != nil {
		return err
	}
	if has {
		return c.ensureLoaded(ctx)
	}

	// 配置中文分词器
	analyzerParams := map[string]any{"type": "chinese"}
	// 定义 Schema: id, kb_id, doc_id, content, vector, metadata
	bm25Function := entity.NewFunction().
		WithName("bm25_auto").
		WithType(entity.FunctionTypeBM25).
		WithInputFields(types.MilvusContentField).
		WithOutputFields(types.MilvusSparseVectorField)
	schema := entity.NewSchema().
		WithField(entity.NewField().
			WithName(types.MilvusIDField).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(64).
			WithIsPrimaryKey(true)).
		WithField(entity.NewField().
			WithName(types.MilvusKnowledgeIDField).
			WithDataType(entity.FieldTypeInt64).
			WithMaxLength(64).
			WithIsPartitionKey(true)). // 多知识库高效过滤
		WithField(entity.NewField().
			WithName(types.MilvusDocumentIDField).
			WithDataType(entity.FieldTypeInt64).
			WithMaxLength(64)).
		WithField(entity.NewField().
			WithName(types.MilvusContentField).
			WithDataType(entity.FieldTypeVarChar).
			WithMaxLength(65535).
			WithEnableAnalyzer(true).
			WithAnalyzerParams(analyzerParams)).
		WithField(entity.NewField().
			WithName(types.MilvusVectorFieldField).
			WithDataType(entity.FieldTypeFloatVector).
			WithDim(types.VectorDim)).
		WithField(entity.NewField().
			WithName(types.MilvusSparseVectorField).
			WithDataType(entity.FieldTypeSparseVector)).
		WithField(entity.NewField().
			WithName(types.MilvusMetadataField).
			WithDataType(entity.FieldTypeJSON)).
		WithFunction(bm25Function)

	// 创建 Collection
	err = c.client.CreateCollection(ctx,
		milvusclient.NewCreateCollectionOption(c.collection, schema),
	)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	if err := c.createIndex(ctx); err != nil {
		return err
	}
	return c.ensureLoaded(ctx)
}

func (c *milvusClient) createIndex(ctx context.Context) error {
	denseIdx := index.NewHNSWIndex(c.denseMetric, 16, 200)
	task, err := c.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(c.collection, types.MilvusVectorFieldField, denseIdx))
	if err != nil {
		return fmt.Errorf("ensure dense vector index: %w", err)
	}
	if err = task.Await(ctx); err != nil {
		return err
	}

	sparseIdx := index.NewSparseInvertedIndex(c.sparseMetric, 0.2)
	task, err = c.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(c.collection, types.MilvusSparseVectorField, sparseIdx))
	if err != nil {
		return fmt.Errorf("ensure sparse vector index: %w", err)
	}

	return task.Await(ctx)
}

func (c *milvusClient) ensureIndex(ctx context.Context, fieldName string, idx index.Index) error {
	indexes, err := c.client.ListIndexes(ctx, milvusclient.NewListIndexOption(c.collection).WithFieldName(fieldName))
	if err != nil {
		return err
	}
	if len(indexes) > 0 {
		return nil
	}

	task, err := c.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(c.collection, fieldName, idx))
	if err != nil {
		return err
	}
	return task.Await(ctx)
}

func (c *milvusClient) ensureLoaded(ctx context.Context) error {
	loadState, err := c.client.GetLoadState(ctx, milvusclient.NewGetLoadStateOption(c.collection))
	if err != nil {
		return err
	}
	if loadState.State == entity.LoadStateLoaded {
		return nil
	}
	task, err := c.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(c.collection))
	if err != nil {
		return err
	}
	return task.Await(ctx)
}

// SearchChunks 向量相似度检索，返回 TopK 个匹配块
func (c *milvusClient) SearchChunks(ctx context.Context, vector []float32, kbID string, topK int) ([]types.SearchResult, error) {
	filter := fmt.Sprintf(`%s == "%s"`, types.MilvusKnowledgeIDField, kbID)

	rs, err := c.client.Search(ctx, milvusclient.NewSearchOption(c.collection, topK, []entity.Vector{entity.FloatVector(vector)}).
		WithANNSField(types.MilvusVectorFieldField).
		WithFilter(filter).
		WithOutputFields(types.MilvusIDField, types.MilvusDocumentIDField, types.MilvusContentField, types.MilvusMetadataField),
	)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}

	results := make([]types.SearchResult, 0, rs[0].ResultCount)
	for i := range rs[0].ResultCount {
		id, _ := rs[0].GetColumn(types.MilvusIDField).GetAsString(i)
		docID, _ := rs[0].GetColumn(types.MilvusDocumentIDField).GetAsString(i)
		content, _ := rs[0].GetColumn(types.MilvusContentField).GetAsString(i)
		score := rs[0].Scores[i]

		results = append(results, types.SearchResult{
			ID:         id,
			DocumentID: docID,
			Content:    content,
			Score:      score,
		})
	}
	return results, nil
}

// GetCollectionStats 获取 collection 统计信息（总向量数等）
func (c *milvusClient) GetCollectionStats(ctx context.Context) (map[string]string, error) {
	stats, err := c.client.GetCollectionStats(ctx, milvusclient.NewGetCollectionStatsOption(c.collection))
	if err != nil {
		return nil, fmt.Errorf("get collection stats: %w", err)
	}
	return stats, nil
}

// BuildKBFilter 辅助函数：将业务侧的 []int 转为 Milvus 的 IN 表达式
func intInFilter(fieldName string, ids []int) string {
	values := lo.Map(ids, func(id int, _ int) string {
		return strconv.Itoa(id)
	})
	return fmt.Sprintf("%s in [%s]", fieldName, strings.Join(values, ", "))
}

func BuildKBFilter(kbIDs []int) string {
	if len(kbIDs) == 0 {
		return ""
	}
	return intInFilter(types.MilvusKnowledgeIDField, kbIDs)
}
