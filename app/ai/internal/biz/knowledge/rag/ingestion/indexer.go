package ingestion

import (
	"ai/internal/biz/types"
	"ai/internal/conf"
	"ai/internal/data"
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino-ext/components/indexer/milvus2"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/milvus-io/milvus/client/v2/column"
	"github.com/milvus-io/milvus/client/v2/entity"
	"github.com/milvus-io/milvus/client/v2/milvusclient"
)

type (
	IndexerNode struct {
		ksc  data.KnowledgeSegmentClient
		l    *log.Helper
		conf *IndexerConfig
	}

	IndexerConfig struct {
		Indexer indexer.Indexer
	}
)

func NewIndexer(ksc data.KnowledgeSegmentClient, l log.Logger, conf *IndexerConfig) (indexer.Indexer, error) {
	if conf == nil {
		return nil, fmt.Errorf("indexer config is nil")
	}
	return &IndexerNode{
		ksc:  ksc,
		l:    log.NewHelper(l, log.WithMessageKey("rag-indexer")),
		conf: conf,
	}, nil
}

func (n *IndexerNode) Store(ctx context.Context, docs []*schema.Document, opts ...indexer.Option) (ids []string, err error) {
	ids, err = n.conf.Indexer.Store(ctx, docs, opts...)
	if err != nil {
		return nil, err
	}
	//info.VectorIDs = ids
	fails := make([]int, 0)
	for i, seg := range docs {
		segID, _ := strconv.Atoi(seg.ID)
		if err := n.ksc.UpdateVectorID(ctx, segID, ids[i]); err != nil {
			n.l.Errorf("failed to update vector id %d: %v", segID, err)
			fails = append(fails, segID)
		}
	}
	if len(fails) > 0 {
		return nil, fmt.Errorf("failed to update vector nil: %v", fails)
	}
	return ids, nil
}

func NewMilvusIndexer(ksc data.KnowledgeSegmentClient, l log.Logger, emb embedding.Embedder, bs *conf.Bootstrap) (indexer.Indexer, error) {
	cfg := bs.Data.Milvus
	dmType := milvus2.MetricType(cfg.MetricType.Dense)
	if dmType == "" {
		dmType = milvus2.COSINE
	}
	smType := milvus2.MetricType(cfg.MetricType.Sparse)
	if smType == "" {
		smType = milvus2.BM25
	}
	ctx := context.Background()
	// 配置向量索引
	vecCfg := &milvus2.VectorConfig{
		Dimension:    1024, // 与 embedding 模型维度匹配
		MetricType:   milvus2.MetricType(cfg.MetricType.Dense),
		IndexBuilder: milvus2.NewHNSWIndexBuilder().WithM(16).WithEfConstruction(200),
		VectorField:  types.MilvusVectorFieldField,
	}
	// 配置稀疏向量索引
	svecCfg := &milvus2.SparseVectorConfig{
		IndexBuilder: milvus2.NewSparseInvertedIndexBuilder().
			WithDropRatioBuild(0.2), // 构建时忽略 20% 的低权重词，减小索引体积
		VectorField: types.MilvusSparseVectorField,
		MetricType:  milvus2.MetricType(cfg.MetricType.Sparse),
		Method:      milvus2.SparseMethodAuto,
	}
	indexer, err := milvus2.NewIndexer(ctx, &milvus2.IndexerConfig{
		ClientConfig: &milvusclient.ClientConfig{
			Address:  cfg.Addr,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Collection:        cfg.Collection,
		Vector:            vecCfg,
		Sparse:            svecCfg,
		Embedding:         emb,
		DocumentConverter: MilvusDocumentConverter(vecCfg, nil),
	})
	if err != nil {
		return nil, err
	}
	return &IndexerNode{
		ksc: ksc,
		l:   log.NewHelper(l, log.WithMessageKey("rag-indexer")),
		conf: &IndexerConfig{
			Indexer: indexer,
		},
	}, nil
}

func MilvusDocumentConverter(vector *milvus2.VectorConfig, sparse *milvus2.SparseVectorConfig) func(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]column.Column, error) {
	return func(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]column.Column, error) {
		docInfo := GetDocumentInfo(ctx)
		docID := docInfo.ID
		kbID := docInfo.KnowledgeID

		ids := make([]string, 0, len(docs))
		docIDs := make([]int64, 0, len(docs))
		kbIDs := make([]int64, 0, len(docs))
		contents := make([]string, 0, len(docs))
		vecs := make([][]float32, 0, len(docs))
		metadatas := make([][]byte, 0, len(docs))
		sparseVecs := make([]entity.SparseEmbedding, 0, len(docs))

		// Determine if we need to handle sparse vectors
		sparseVectorField := ""
		if sparse != nil && sparse.Method == milvus2.SparseMethodPrecomputed {
			sparseVectorField = sparse.VectorField
		}

		// Determine if we need to handle dense vectors
		denseVectorField := ""
		if vector != nil {
			denseVectorField = vector.VectorField
		}

		for idx, doc := range docs {
			ids = append(ids, doc.ID)
			docIDs = append(docIDs, int64(docID))
			kbIDs = append(kbIDs, int64(kbID))
			contents = append(contents, doc.Content)

			var sourceVec []float64
			if len(vectors) == len(docs) {
				sourceVec = vectors[idx]
			} else {
				sourceVec = doc.DenseVector()
			}

			// Dense vector is required when vectorField is set (dense-only or hybrid mode).
			if denseVectorField != "" {
				if len(sourceVec) == 0 {
					return nil, fmt.Errorf("vector data missing for document %d (id: %s)", idx, doc.ID)
				}
				vec := make([]float32, len(sourceVec))
				for i, v := range sourceVec {
					vec[i] = float32(v)
				}
				vecs = append(vecs, vec)
			}

			if sparseVectorField != "" {
				sv := doc.SparseVector()
				se, err := toMilvusSparseEmbedding(sv)
				if err != nil {
					return nil, fmt.Errorf("failed to convert sparse vector for document %d: %w", idx, err)
				}
				sparseVecs = append(sparseVecs, se)
			}

			metadata, err := sonic.Marshal(doc.MetaData)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal metadata: %w", err)
			}
			metadatas = append(metadatas, metadata)
		}

		columns := []column.Column{
			column.NewColumnVarChar(types.MilvusIDField, ids),
			column.NewColumnInt64(types.MilvusDocumentIDField, docIDs),
			column.NewColumnInt64(types.MilvusKnowledgeIDField, kbIDs),
			column.NewColumnVarChar(types.MilvusContentField, contents),
			column.NewColumnJSONBytes(types.MilvusMetadataField, metadatas),
		}

		if denseVectorField != "" {
			dim := 0
			if len(vecs) > 0 {
				dim = len(vecs[0])
			}
			columns = append(columns, column.NewColumnFloatVector(denseVectorField, dim, vecs))
		}

		if sparseVectorField != "" {
			// SparseFloatVector column does not typically require specific dimension argument in insert
			columns = append(columns, column.NewColumnSparseVectors(sparseVectorField, sparseVecs))
		}

		return columns, nil
	}
}

func toMilvusSparseEmbedding(sv map[int]float64) (entity.SparseEmbedding, error) {
	if len(sv) == 0 {
		return entity.NewSliceSparseEmbedding([]uint32{}, []float32{})
	}

	indices := make([]int, 0, len(sv))
	for k := range sv {
		indices = append(indices, k)
	}

	sort.Ints(indices)

	uint32Indices := make([]uint32, len(indices))
	values := make([]float32, len(indices))

	for i, idx := range indices {
		if idx < 0 {
			return nil, fmt.Errorf("negative sparse index: %d", idx)
		}
		uint32Indices[i] = uint32(idx)
		values[i] = float32(sv[idx])
	}

	return entity.NewSliceSparseEmbedding(uint32Indices, values)
}
