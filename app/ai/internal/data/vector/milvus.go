package vector

import (
	"ai/internal/biz/types"
	"ai/internal/conf"
	"context"
	"fmt"
	"strings"

	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/index"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
	"github.com/samber/lo"
)

type VectorStore interface {
	// DeleteByDocID 按业务文档 ID 删除所有切片 (极度常用：用户在界面上点击"删除该文件"时触发)
	DeleteByDocID(ctx context.Context, kbID int, docID int) error

	// CountChunksByDocID 获取某文档切片数量/详情 (用于后台管理或对账)
	CountChunksByDocID(ctx context.Context, kbID int, docID int) (int64, error)

	// SearchKnowledge 搜索知识库，返回 topK 个最相关的切片 ID
	SearchKnowledge(ctx context.Context, kbID string, queryVectors [][]float32, topK int) ([]string, error)

	// Delete 删除切片
	Delete(ctx context.Context, ids []string) error

	// GetContentByIDs 获取切片内容
	GetContentByIDs(ctx context.Context, ids []string) ([]string, error)
}

func NewMilvusClient(bs *conf.Bootstrap) (VectorStore, error) {
	cfg := bs.Data.Milvus
	client, err := milvusclient.New(context.Background(), &milvusclient.ClientConfig{
		Address:  cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create milvus client: %w", err)
	}
	return &milvusClient{
		client:     client,
		collection: cfg.Collection,
	}, nil
}

type milvusClient struct {
	client     *milvusclient.Client
	collection string
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

func (c *milvusClient) DeleteByDocID(ctx context.Context, kbID int, docID int) error {
	expr := fmt.Sprintf("%s = '%d' AND %s = '%d'", types.MilvusKnowledgeIDField, kbID, types.MilvusDocumentIDField, docID)
	_, err := c.client.Delete(ctx, milvusclient.NewDeleteOption(c.collection).
		WithExpr(expr))
	if err != nil {
		return fmt.Errorf("failed to delete chunks for doc %d: %w", docID, err)
	}
	return nil
}

func (c *milvusClient) CountChunksByDocID(ctx context.Context, kbID int, docID int) (int64, error) {
	expr := fmt.Sprintf("%s = '%d' AND %s = '%d'", types.MilvusKnowledgeIDField, kbID, types.MilvusDocumentIDField, docID)
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
	option := milvusclient.NewDeleteOption(c.collection).
		WithStringIDs(types.MilvusIDField, ids)

	_, err := c.client.Delete(ctx, option)
	return err
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
		return nil
	}

	// 配置中文分词器
	analyzerParams := map[string]any{"type": "chinese"}
	// 定义 Schema: id, kb_id, doc_id, content, vector, metadata
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
			WithDataType(entity.FieldTypeJSON))

	// 创建 Collection
	err = c.client.CreateCollection(ctx,
		milvusclient.NewCreateCollectionOption(c.collection, schema),
	)
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// 创建向量索引（HNSW，适合单机高精度场景）
	idx := index.NewHNSWIndex(entity.COSINE, 16, 200)
	_, err = c.client.CreateIndex(ctx, milvusclient.NewCreateIndexOption(c.collection, types.MilvusVectorFieldField, idx))
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	_, err = c.client.LoadCollection(ctx, milvusclient.NewLoadCollectionOption(c.collection))
	return err
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
func BuildKBFilter(kbIDs []int) string {
	if len(kbIDs) == 0 {
		return ""
	}

	// 给每个 ID 加上单引号
	quotedIDs := make([]string, len(kbIDs))
	for i, id := range kbIDs {
		quotedIDs[i] = fmt.Sprintf("'%d'", id)
	}

	// 拼接成: kb_id in ['id1', 'id2']
	return fmt.Sprintf("%s in [%s]", types.MilvusKnowledgeIDField, strings.Join(quotedIDs, ", "))
}
